package validate

import (
	"errors"
	"fmt"
	zhongwen "github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	zh_translations "github.com/go-playground/validator/v10/translations/zh"
	"reflect"
	"strings"
)

var validate *validator.Validate
var trans ut.Translator

// init函数在程序启动时初始化必要的包级变量。
// 这包括设置中文翻译器和初始化验证器，以便在全局范围内使用。
func init() {
	// 创建一个新的中文处理实例。
	zh := zhongwen.New()
	// 创建一个通用翻译器实例，使用中文作为唯一语言。
	uni := ut.New(zh, zh)
	// 获取一个中文翻译器实例，忽略可能的错误。
	trans, _ = uni.GetTranslator("zh")
	// 创建一个新的验证器实例。
	validate = validator.New()
	// 注册一个函数来处理结构体字段的标签，以便在验证错误时使用更友好的字段名称。
	validate.RegisterTagNameFunc(func(field reflect.StructField) string {
		label := field.Tag.Get("label")
		if label == "" {
			return field.Name
		}
		return label
	})
	// 注册默认的翻译消息，以便在验证错误时使用。
	_ = zh_translations.RegisterDefaultTranslations(validate, trans)
}

// translate函数接收一个错误对象，如果它是验证错误，则将其翻译成中文。
// 这个函数返回一个翻译后的错误对象。
func translate(errs error) error {
	// 初始化一个字符串切片来存储翻译后的错误消息。
	var errList []string
	// 定义一个变量来存储验证错误。
	var v validator.ValidationErrors
	switch {
	case errors.As(errs, &v):
		for _, e := range v {
			errList = append(errList, e.Translate(trans))
		}
		// 将所有翻译后的错误消息合并成一个字符串并返回。
		return fmt.Errorf(strings.Join(errList, "|"))
	default:
		return errs
	}
}

func Validate[T any](r T) error {
	if err := validate.Struct(r); err != nil {
		return translate(err)
	}
	return nil
}
