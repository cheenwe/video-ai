# ONNX 推理 Worker

平台侧（Go）负责：RTSP 取流、JPEG 解码、letterbox、归一化、NCHW 张量构建、
输出解码与 NMS。本 worker 只负责 **ONNX Runtime 会话**，因此主二进制不需要
CGO / ORT 原生库。

## 协议

stdin/stdout 上的帧格式：4 字节大端长度 + JSON 头，随后是可选负载
（长度在头的 `n` 字段）。

| 方向 | 帧 | 说明 |
| --- | --- | --- |
| → | `{"op":"init"}` + 配置 JSON | `model_path` / `providers` / `input_name` / `input_size` / `classes` |
| ← | `{"ok":true,"inputs":[...],"outputs":[...]}` 或 `{"ok":false,"error":...}` | 会话元数据 / 失败原因 |
| → | `{"op":"infer","n":<字节数>,"name":"images","shape":[1,3,H,W]}` + float32 LE 张量 | 每帧一次 |
| ← | `{"ok":true,"outputs":[...]}` + 每个输出一段长度前缀 float32 LE 负载 | 原始输出，后处理在 Go 侧 |
| → | `{"op":"close"}` | 结束 |

## 安装与运行

```bash
pip install -r requirements.txt   # onnxruntime + numpy
python3 worker.py                 # 平台会自动拉起；也可用 BASE_VIDEO_AI_ONNX_WORKER 指向自定义命令
```

## 说明

- 输入张量由平台按 `input_size`（默认 640×640）letterbox + /255 归一化；
  输出按 YOLO detect 布局（`[1, 4+nc, N]` 或 `[1, N, 4+nc]`）在 Go 侧解码。
- 非 YOLO 布局的模型需要在 Go 侧 `postprocess` 中扩展解码（`internal/videoai/postprocess.go`）。
- worker 崩溃或超时（15s）后，平台会标记会话死亡并在下一 tick 重启。
