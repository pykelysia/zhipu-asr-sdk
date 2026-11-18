# zhipu-asr-sdk

智谱语音转写（ASR）官方协议的轻量 Golang SDK，零第三方依赖，仅使用标准库。

协议与参数以 [docs/zhipu-asr-api.md](./docs/zhipu-asr-api.md) 为准。

## 特性

- 同步转写：上传音频文件（`.wav` / `.mp3`，≤25MB、≤30s）或 Base64，返回结构化 JSON
- 流式转写：SSE 增量返回 `delta` 片段与最终 `text`，自动处理 `data: [DONE]` 终止标记
- 完整请求参数支持：`model` / `prompt` / `hotwords` / `request_id` / `user_id` / `stream`
- 结构化错误：非 2xx 响应解析为 `*asr.APIError`（含 HTTP 状态码、错误码、消息）
- 可注入 `*http.Client`，自定义超时、代理、传输层

## 安装

```bash
go get github.com/pyke/zhipu-asr-sdk
```

## 快速开始

```go
client := asr.NewClient(os.Getenv("ZHIPU_API_KEY"))

resp, err := client.Transcribe(context.Background(), asr.TranscriptionRequest{
    Model:    asr.ModelGLMASR,
    FilePath: "audio.wav",
})
if err != nil {
    log.Fatal(err)
}
fmt.Println(resp.Text)
```

流式转写：

```go
stream, err := client.TranscribeStream(context.Background(), asr.TranscriptionRequest{
    Model:    asr.ModelGLMASR,
    FilePath: "audio.mp3",
})
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for stream.Next() {
    ev := stream.Event()
    if ev.Type == asr.EventTranscriptTextDelta {
        fmt.Print(ev.Delta)
    }
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}
```

更多用法见 [examples/basic](./examples/basic/main.go)。

## License

MIT
