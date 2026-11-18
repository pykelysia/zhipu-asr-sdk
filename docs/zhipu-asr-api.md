# 智谱语音转写（zhipu-asr）接口文档

## 1. 概述

智谱语音识别（ASR）将音频转换为文本。请求为 `multipart/form-data` 表单（上传音频文件或 Base64），支持同步 JSON 与流式 SSE 两种响应。apiCenter 网关已将该协议以 `zhipu-asr` 协议名接入，客户端形态与官方完全一致（multipart 原样透传）。

## 2. 端点与认证

| 环境 | 端点 | 认证 |
|---|---|---|
| 智谱官方 | `POST https://open.bigmodel.cn/api/paas/v4/audio/transcriptions` | `Authorization: Bearer <智谱 API Key>` |

## 3. 请求参数（multipart/form-data）

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `file` | binary | 与 `file_base64` 二选一 | 音频文件，支持 `.wav` / `.mp3`；≤25MB、时长 ≤30 秒。同时传两者时以 `file` 为准 |
| `file_base64` | string | 与 `file` 二选一 | 音频 Base64 编码（解码后同样受 ≤25MB / ≤30 秒约束） |
| `model` | string | ✅ | 模型编码，枚举值 `glm-asr-2512`。网关侧此值需与所配置的模型 ID 一致（透传不改名） |
| `prompt` | string | — | 上下文提示：长音频分段场景提供之前的转录结果，建议 <8000 字 |
| `hotwords` | string[] | — | 热词表（人名、地名等），最多 100 个，提升特定领域词汇识别率。multipart 中重复发送同名字段 |
| `stream` | boolean | — | 默认 `false`。`true` 时以 Event Stream 逐块返回，结束时发送 `data: [DONE]` |
| `request_id` | string | — | 请求唯一标识，6–64 字符，建议 UUID；未传则由服务端自动生成 |
| `user_id` | string | — | 终端用户唯一 ID，6–128 字符，用于平台滥用行为干预 |

## 4. 响应

### 4.1 同步（`application/json`）

```json
{
  "id": "任务 ID",
  "created": 1730000000,
  "request_id": "请求唯一标识（回显）",
  "model": "glm-asr-2512",
  "text": "完整转录文本"
}
```

### 4.2 流式（`text/event-stream`，`stream=true`）

每条消息为 `data: {JSON}\n\n`，以 `type` 字段区分事件：

| 事件 `type` | 字段 | 含义 |
|---|---|---|
| `transcript.text.delta` | `delta` | 增量转录文本片段 |
| `transcript.text.done` | `text` | 完整转录结果，流结束前最后一条业务事件 |

事件对象同样携带 `id` / `created` / `request_id` / `model`。流末尾发送 `data: [DONE]` 作为终止标记，随后连接关闭。

## 5. 错误

HTTP 非 2xx 时返回：

```json
{ "error": { "code": "错误码", "message": "错误描述" } }
```

网关侧错误结构同 OpenAI 风格（`{ "error": { "message", "type", "code" } }`）：无效 Key 401、模型不存在 404、白名单外模型 403、额度用尽 429、上游不可达/上游 401 → 502、上游其余错误码原样透传。

## 6. 约束与限制

- 音频：≤25MB、时长 ≤30 秒、格式 `.wav` / `.mp3`
- `prompt` 建议 <8000 字；`hotwords` ≤100 个
- `request_id` 6–64 字符；`user_id` 6–128 字符
- 无 usage/token 返回：响应不含用量字段，apiCenter 对该协议**暂不计费**（日志 `usage_known=0`，token/成本记 0，不扣 Key 额度）
- 端点互斥：对话端点（`/v1/chat/completions` 等）不能调 ASR 模型；ASR 端点不能调对话模型
- `/v1/audio/transcriptions`（OpenAI 风格路径）在网关中预留未实现，留给未来的 OpenAI 兼容转写协议

## 7. 调用示例

非流式（官方）：

```bash
curl -X POST https://open.bigmodel.cn/api/paas/v4/audio/transcriptions \
  -H "Authorization: Bearer $ZHIPU_API_KEY" \
  -F file=@audio.wav \
  -F model=glm-asr-2512 \
  -F 'prompt=会议记录' \
  -F 'hotwords=张三' -F 'hotwords=李四'
```

同步响应示例：

```json
{ "id": "asr_8f3c", "created": 1760000000, "request_id": "req-42", "model": "glm-asr-2512", "text": "转录(2热词):会议记录" }
```

流式响应示例：

```
data: {"id":"asr_8f3c","created":1760000000,"request_id":"req-42","model":"glm-asr-2512","type":"transcript.text.delta","delta":"会议"}

data: {"id":"asr_8f3c","created":1760000000,"request_id":"req-42","model":"glm-asr-2512","type":"transcript.text.delta","delta":"记录"}

data: {"id":"asr_8f3c","created":1760000000,"request_id":"req-42","model":"glm-asr-2512","type":"transcript.text.done","text":"会议记录"}

data: [DONE]
```

## 8. SDK 实现要点备忘

- multipart 构造：`file`（binary part）与 `file_base64`（文本 part）二选一；`hotwords` 数组以重复字段发送，不要序列化成 JSON 数组字符串
- 流式解析：按行读 SSE，取 `data:` 前缀；`[DONE]` 为终止标记；事件以 `type` 字段区分（payload 内，非 `event:` 命名行）
- 终止与超时：连接关闭即流结束；建议客户端设读超时并对 `delta` 做拼接缓冲
- 错误恢复：`request_id` 可用于问题排查回溯；上游 401/网络错误需在 SDK 层重试或抛出明确异常

---

以上内容以 [智谱官方文档为准](https://docs.bigmodel.cn/api-reference/%E6%A8%A1%E5%9E%8B-api/%E8%AF%AD%E9%9F%B3%E8%BD%AC%E6%96%87%E6%9C%AC)