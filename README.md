# zhipu-asr-sdk

智谱语音转写（ASR）官方协议的轻量 Golang SDK，零第三方依赖，仅使用标准库。

协议与参数以 [docs/zhipu-asr-api.md](./docs/zhipu-asr-api.md) 为准。

## 特性

- 同步转写：上传音频文件（`.wav` / `.mp3`，≤25MB、时长 ≤30s）或 Base64，返回结构化 JSON
- 流式转写：SSE 增量返回 `delta` 片段与最终 `text`，自动处理 `data: [DONE]` 终止标记
- 完整请求参数支持：`model` / `prompt` / `hotwords` / `request_id` / `user_id` / `stream`
- 结构化错误：非 2xx 响应解析为 `*asr.APIError`（含 HTTP 状态码、错误码、消息、请求 ID）
- 可注入 `*http.Client`、覆盖 BaseURL，自定义超时、代理、传输层

## 安装

```bash
go get github.com/pyke/zhipu-asr-sdk
```

## 快速开始

### 同步转写

```go
client := asr.NewClient(os.Getenv("ZHIPU_API_KEY"))

resp, err := client.Transcribe(ctx, asr.TranscriptionRequest{
    Model:    asr.ModelGLMASR2512,
    FilePath: "audio.wav",
    Prompt:   "会议记录",
    Hotwords: []string{"张三", "李四"},
})
if err != nil {
    log.Fatal(err)
}
fmt.Println(resp.Text)
```

### 流式转写

```go
stream, err := client.TranscribeStream(ctx, asr.TranscriptionRequest{
    Model:    asr.ModelGLMASR2512,
    FilePath: "audio.mp3",
})
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for stream.Next() {
    if ev := stream.Event(); ev.Type == asr.EventTranscriptTextDelta {
        fmt.Print(ev.Delta)
    }
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}
```

也可以直接用 `stream.FullText()` 消费整条流并拿最终文本。

## 请求参数

`TranscriptionRequest` 的音频来源三选一，优先级 `File` > `FilePath` > `FileBase64`
（与官方“`file` 与 `file_base64` 同时传时以 `file` 为准”一致）：

| 字段 | 说明 |
|---|---|
| `File` + `FileName` | 内存中的音频内容（`io.Reader`）及文件名 |
| `FilePath` | 本地音频路径，文件名自动取基名 |
| `FileBase64` | 音频 Base64 编码字符串 |
| `Model` | 必填，当前枚举值 `asr.ModelGLMASR2512`（`glm-asr-2512`） |
| `Prompt` | 上下文提示，长音频分段时传之前的转录结果，建议 <8000 字 |
| `Hotwords` | 热词表，最多 100 个，以重复的同名 multipart 字段发送 |
| `Stream` | 同步接口会强制忽略；`TranscribeStream` 自动置 `true` |
| `RequestID` | 请求唯一标识，6–64 字符，未传由服务端生成 |
| `UserID` | 终端用户唯一 ID，6–128 字符 |

## 客户端配置

`WithBaseURL` 覆盖默认 API 根地址（默认 `https://open.bigmodel.cn/api/paas/v4`），填到含版本前缀的同一层级，SDK 会固定拼接 `/audio/transcriptions`：

```go
client := asr.NewClient(
    os.Getenv("ZHIPU_API_KEY"),
    asr.WithBaseURL("https://your-gateway.example.com/api/paas/v4"), // 填到版本前缀层级
    asr.WithHTTPClient(&http.Client{Timeout: 60 * time.Second}),
)
```

## 错误处理

HTTP 非 2xx 时返回 `*asr.APIError`：

```go
var apiErr *asr.APIError
if errors.As(err, &apiErr) {
    log.Printf("status=%d code=%s message=%s request_id=%s",
        apiErr.StatusCode, apiErr.Code, apiErr.Message, apiErr.RequestID)
}
```

常见状态码：401 无效 Key、404 模型不存在、403 白名单外模型、429 额度用尽、502 上游不可达。

## 示例

可运行的完整示例见 [examples/basic](./examples/basic/main.go)：

```bash
ZHIPU_API_KEY=xxx go run ./examples/basic -file audio.wav -stream -hotwords 张三,李四
```

## License

MIT
