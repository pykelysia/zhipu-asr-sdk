// 示例：智谱语音转写的同步与流式调用。
//
// 运行前设置环境变量 ZHIPU_API_KEY，并准备一段 ≤30 秒的 .wav/.mp3 音频：
//
//	ZHIPU_API_KEY=xxx go run ./examples/basic -file audio.wav -stream
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	asr "github.com/pykelysia/zhipu-asr-sdk"
)

func main() {
	apiKey := os.Getenv("ZHIPU_API_KEY")
	if apiKey == "" {
		log.Fatal("请先设置环境变量 ZHIPU_API_KEY")
	}
	file := flag.String("file", "audio.wav", "音频文件路径（.wav/.mp3，≤25MB、≤30 秒）")
	stream := flag.Bool("stream", false, "使用流式接口，逐块打印增量文本")
	hotwords := flag.String("hotwords", "", "热词，逗号分隔，如：张三,李四")
	flag.Parse()

	req := asr.TranscriptionRequest{
		Model:    asr.ModelGLMASR2512,
		FilePath: *file,
	}
	if *hotwords != "" {
		req.Hotwords = splitHotwords(*hotwords)
	}

	client := asr.NewClient(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if *stream {
		runStream(ctx, client, req)
		return
	}
	runSync(ctx, client, req)
}

func runSync(ctx context.Context, client *asr.Client, req asr.TranscriptionRequest) {
	resp, err := client.Transcribe(ctx, req)
	if err != nil {
		log.Fatalf("同步转写失败: %v", err)
	}
	fmt.Printf("文本: %s\n任务: %s（request_id=%s, model=%s, created=%d）\n",
		resp.Text, resp.ID, resp.RequestID, resp.Model, resp.Created)
}

func runStream(ctx context.Context, client *asr.Client, req asr.TranscriptionRequest) {
	stream, err := client.TranscribeStream(ctx, req)
	if err != nil {
		log.Fatalf("流式转写失败: %v", err)
	}
	defer stream.Close()

	for stream.Next() {
		if ev := stream.Event(); ev.Type == asr.EventTranscriptTextDelta {
			fmt.Print(ev.Delta)
		}
	}
	if err := stream.Err(); err != nil {
		log.Fatalf("\n流式转写出错: %v", err)
	}
	fmt.Println()
}

func splitHotwords(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
