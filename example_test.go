package asr_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	asr "github.com/pykelysia/zhipu-asr-sdk"
)

// 同步转写：上传本地音频文件，一次性返回完整文本。
func ExampleClient_Transcribe() {
	client := asr.NewClient(os.Getenv("ZHIPU_API_KEY"))

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	resp, err := client.Transcribe(ctx, asr.TranscriptionRequest{
		Model:    asr.ModelGLMASR2512,
		FilePath: "meeting.wav", // ≤25MB、时长 ≤30 秒
		Prompt:   "会议记录",
		Hotwords: []string{"张三", "李四"},
	})
	if err != nil {
		var apiErr *asr.APIError
		switch {
		case errors.As(err, &apiErr):
			log.Fatalf("服务端错误 %d: %s", apiErr.StatusCode, apiErr.Message)
		default:
			log.Fatal(err)
		}
	}
	fmt.Println(resp.Text)
}

// 流式转写：逐块打印增量文本，[DONE] 由 SDK 内部处理。
func ExampleClient_TranscribeStream() {
	client := asr.NewClient(os.Getenv("ZHIPU_API_KEY"))

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	stream, err := client.TranscribeStream(ctx, asr.TranscriptionRequest{
		Model:      asr.ModelGLMASR2512,
		FileBase64: base64OfAudio(), // 或使用 FilePath / File
	})
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()

	var full strings.Builder
	for stream.Next() {
		if ev := stream.Event(); ev.Type == asr.EventTranscriptTextDelta {
			full.WriteString(ev.Delta)
		}
	}
	if err := stream.Err(); err != nil {
		log.Fatal(err)
	}
	fmt.Println(full.String())
}

func base64OfAudio() string { return "" /* 音频 Base64 编码 */ }
