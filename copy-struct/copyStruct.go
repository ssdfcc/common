package copy_struct

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// typeCache 用于缓存类型映射，避免重复计算
var (
	typeCache sync.Map
	bufPool   = sync.Pool{New: func() any { return new(bytes.Buffer) }}
)

// fieldMapping 定义了源和目标字段之间的映射关系
type fieldMapping struct {
	srcOffset  uintptr
	destOffset uintptr
	timeLayout string
	isNested   bool
	converter  func(src unsafe.Pointer, dest unsafe.Pointer) error
}

// typePair 用于在缓存中存储源和目标类型的组合
type typePair struct {
	src  reflect.Type
	dest reflect.Type
}

// CopyStruct 是结构体复制的主入口函数
// 它接受源和目标结构体作为参数，并执行深拷贝操作
func CopyStruct(src interface{}, dest interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("copy panic: %v", r)
		}
	}()
	if src == nil || dest == nil {
		return fmt.Errorf("source or destination is nil")
	}
	// 其他代码
	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.Elem().Kind() != reflect.Struct {
		return errors.New("destination must be a pointer to struct")
	}

	srcVal := reflect.ValueOf(src)
	if srcVal.Kind() == reflect.Ptr {
		srcVal = srcVal.Elem()
	}

	return copyStructRecursive(srcVal, destVal.Elem())
}

// copyStructRecursive 递归地复制结构体
func copyStructRecursive(srcVal, destVal reflect.Value) error {
	if srcVal.Kind() != reflect.Struct {
		return errors.New("source must be a struct")
	}

	cacheKey := typePair{
		src:  srcVal.Type(),
		dest: destVal.Type(),
	}

	cached, ok := typeCache.Load(cacheKey)
	var mappings []fieldMapping

	if !ok {
		mappings = createTypeMapping(srcVal, destVal)
		typeCache.Store(cacheKey, mappings)
	} else {
		mappings = cached.([]fieldMapping)
	}

	return applyFieldMappings(srcVal, destVal, mappings)
}

// createTypeMapping 创建源和目标结构体的字段映射
func createTypeMapping(srcVal, destVal reflect.Value) []fieldMapping {
	srcType := srcVal.Type()
	destType := destVal.Type()
	mappings := make([]fieldMapping, 0, srcType.NumField())

	for i := 0; i < srcType.NumField(); i++ {
		srcField := srcType.Field(i)
		if destField, ok := destType.FieldByName(srcField.Name); ok {
			if !destVal.FieldByName(srcField.Name).CanSet() {
				continue
			}
			mappings = append(mappings, analyzeFieldMapping(srcField, destField))
		}
	}
	return mappings
}

// analyzeFieldMapping 分析并返回两个结构体字段之间的映射关系
func analyzeFieldMapping(srcField, destField reflect.StructField) fieldMapping {
	mapping := fieldMapping{
		srcOffset:  srcField.Offset,
		destOffset: destField.Offset,
	}

	// 解析时间格式标签
	if tag := srcField.Tag.Get("to"); tag != "" {
		if strings.HasPrefix(tag, "timeFormat:") {
			mapping.timeLayout = strings.SplitN(tag, ":", 2)[1]
		} else if tag == "timeString" {
			mapping.timeLayout = "2006-01-02 15:04:05"
		}
	}

	// 验证时间字段类型
	if mapping.timeLayout != "" {
		srcType := srcField.Type
		if srcType.Kind() == reflect.Ptr {
			srcType = srcType.Elem()
		}
		if srcType != reflect.TypeOf(time.Time{}) || destField.Type.Kind() != reflect.String {
			mapping.timeLayout = ""
		}
	}

	// 检查嵌套结构体
	if isNestedType(srcField.Type, destField.Type) {
		mapping.isNested = true
	}

	// 生成高效转换器
	mapping.converter = createConverter(srcField.Type, destField.Type, mapping)
	// 强制检查基础类型转换
	if isBasicToStringConvertible(srcField.Type, destField.Type) {
		mapping.converter = createBasicToStringConverter(srcField.Type)
	} else if srcField.Type.ConvertibleTo(destField.Type) {
		mapping.converter = createBasicConverter(srcField.Type, destField.Type)
	}
	return mapping
}

// applyFieldMappings 应用字段映射，将源结构体的字段值复制到目标结构体
func applyFieldMappings(srcVal, destVal reflect.Value, mappings []fieldMapping) error {
	srcPtr := unsafe.Pointer(srcVal.UnsafeAddr())
	destPtr := unsafe.Pointer(destVal.UnsafeAddr())

	for _, m := range mappings {
		if m.converter != nil {
			if err := m.converter(unsafe.Add(srcPtr, m.srcOffset), unsafe.Add(destPtr, m.destOffset)); err != nil {
				return err
			}
		}
	}
	return nil
}

// createConverter 根据字段映射创建相应的转换函数
func createConverter(srcType, destType reflect.Type, m fieldMapping) func(unsafe.Pointer, unsafe.Pointer) error {
	switch {
	case m.timeLayout != "":
		return createTimeConverter(srcType, m.timeLayout)

	case m.isNested:
		return createNestedConverter(srcType, destType)

	default:
		return createBasicConverter(srcType, destType)
	}
}

// createTimeConverter 创建时间格式转换函数
func createTimeConverter(srcType reflect.Type, layout string) func(unsafe.Pointer, unsafe.Pointer) error {
	isPtr := srcType.Kind() == reflect.Ptr

	return func(src, dest unsafe.Pointer) error {
		// 检查空指针
		if isPtr {
			timePtr := *(*unsafe.Pointer)(src)
			if timePtr == nil {
				return nil
			}
			src = timePtr
		}

		t := *(*time.Time)(src)
		buf := bufPool.Get().(*bytes.Buffer)
		buf.Reset()
		buf.WriteString(t.Format(layout))
		*(*string)(dest) = buf.String()
		bufPool.Put(buf)
		return nil
	}
}

// createNestedConverter 创建嵌套结构体的转换函数
func createNestedConverter(srcType, destType reflect.Type) func(unsafe.Pointer, unsafe.Pointer) error {
	srcIsPtr := srcType.Kind() == reflect.Ptr
	destIsPtr := destType.Kind() == reflect.Ptr

	return func(src, dest unsafe.Pointer) error {
		// 处理源字段
		var srcVal reflect.Value
		if srcIsPtr {
			srcPtr := *(*unsafe.Pointer)(src)
			if srcPtr == nil {
				return nil
			}
			srcVal = reflect.NewAt(srcType.Elem(), srcPtr).Elem()
		} else {
			srcVal = reflect.NewAt(srcType, src).Elem()
		}

		// 处理目标字段
		var destVal reflect.Value
		if destIsPtr {
			// 目标是指针类型
			destPtr := (*unsafe.Pointer)(dest)
			if *destPtr == nil {
				*destPtr = unsafe.Pointer(reflect.New(destType.Elem()).Pointer())
			}
			destVal = reflect.NewAt(destType.Elem(), *destPtr).Elem()
		} else {
			// 目标是非指针结构体
			destVal = reflect.NewAt(destType, dest).Elem()
		}

		return copyStructRecursive(srcVal, destVal)
	}
}

// createBasicConverter 创建基本类型的转换函数
func createBasicConverter(srcType, destType reflect.Type) func(unsafe.Pointer, unsafe.Pointer) error {
	if srcType.ConvertibleTo(destType) {
		return func(src, dest unsafe.Pointer) error {
			srcVal := reflect.NewAt(srcType, src).Elem()
			destVal := reflect.NewAt(destType, dest).Elem()
			destVal.Set(srcVal.Convert(destType))
			return nil
		}
	}

	if isBasicToStringConvertible(srcType, destType) {
		return createBasicToStringConverter(srcType)
	}

	return nil // 静默跳过
}

// isBasicToStringConvertible 检查源类型是否可以转换为字符串
func isBasicToStringConvertible(srcType, destType reflect.Type) bool {
	if destType.Kind() != reflect.String {
		return false
	}

	switch srcType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.Bool:
		return true
	case reflect.Ptr:
		return isBasicToStringConvertible(srcType.Elem(), destType)
	default:
		return false
	}
}

// createBasicToStringConverter 创建基本类型到字符串的转换函数
func createBasicToStringConverter(srcType reflect.Type) func(unsafe.Pointer, unsafe.Pointer) error {
	return func(src, dest unsafe.Pointer) error {
		// 处理多级指针
		for srcType.Kind() == reflect.Ptr {
			ptr := *(*unsafe.Pointer)(src)
			if ptr == nil {
				return nil
			}
			src = ptr
			srcType = srcType.Elem()
		}

		buf := bufPool.Get().(*bytes.Buffer)
		buf.Reset()

		switch srcType.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val := *(*int64)(src)
			buf.WriteString(strconv.FormatInt(val, 10))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			val := *(*uint64)(src)
			buf.WriteString(strconv.FormatUint(val, 10))
		case reflect.Float32:
			val := *(*float32)(src)
			buf.WriteString(strconv.FormatFloat(float64(val), 'g', -1, 32))
		case reflect.Float64:
			val := *(*float64)(src)
			buf.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
		case reflect.Bool:
			val := *(*bool)(src)
			buf.WriteString(strconv.FormatBool(val))
		default:
			return nil
		}

		*(*string)(dest) = buf.String()
		bufPool.Put(buf)
		return nil
	}
}

// isNestedType 检查类型是否为嵌套结构体
func isNestedType(src, dest reflect.Type) bool {
	isSrcStruct := src.Kind() == reflect.Struct || (src.Kind() == reflect.Ptr && src.Elem().Kind() == reflect.Struct)
	isDestStruct := dest.Kind() == reflect.Struct || (dest.Kind() == reflect.Ptr && dest.Elem().Kind() == reflect.Struct)
	return isSrcStruct && isDestStruct
}
