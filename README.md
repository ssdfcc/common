### 项目概述

本项目提供了一个结构体复制工具 `CopyStruct`，能够将一个结构体的字段值高效地复制到另一个结构体中。它支持嵌套结构体、时间格式转换以及基本类型之间的转换。此外，还包含一个简单的 HTTP 响应处理函数。

### 目录结构

```
.
├── copy-struct
│   └── CopyStruct.go
└── response
    └── response.go
```


### 功能模块

#### 1. 结构体复制工具 (`copy-struct/CopyStruct.go`)

- **主要功能**：实现结构体之间的深度复制。
- **特性**：
    - 支持嵌套结构体的复制。
    - 支持时间格式的自定义转换（通过结构体标签配置）。
    - 支持基本类型的转换。
    - 使用缓存优化性能。

- **使用方法**：

```go
import "github.com/ssdfcc/common/copy-struct"

// 示例：将 sourceStruct 的值复制到 destinationStruct
err := copy_struct.CopyStruct(&sourceStruct, &destinationStruct)
if err != nil {
    // 处理错误
}
```


- **注意事项**：
    - 源结构体和目标结构体的字段名称需要一致。
    - 目标结构体必须是指向结构体的指针。
    - 时间字段可以通过结构体标签 `to` 来指定格式，例如：`to:"timeFormat:2006-01-02"` 或 `to:"timeString"`。

#### 2. HTTP 响应处理 (`response/response.go`)

- **主要功能**：封装了 go-zero HTTP 响应的处理逻辑，简化 API 返回结果的构建。
- **特性**：
    - 统一的响应格式，包含状态码、消息和数据。
    - 自动处理错误情况。

- **使用方法**：

```go
import "github.com/ssdfcc/common/response"

// 示例：返回成功的响应
response.Response(w, resultData, nil)

// 示例：返回带有错误信息的响应
response.Response(w, nil, someError)
```


### 许可证

本项目采用 MIT 许可证，详情请参见 [LICENSE](LICENSE) 文件。