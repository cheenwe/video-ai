#!/usr/bin/env python3
"""ONNX Runtime inference worker for the video-ai platform.

The Go service owns frame intake, pre-processing and post-processing; this
worker owns the ONNX Runtime session. Frames arrive as NCHW float32 tensors
over stdin, outputs leave as raw float32 tensors over stdout. Every message
is a length-prefixed (4-byte big-endian) JSON header followed by an optional
payload whose length is carried in the header's "n" field.

Protocol:
  -> {"op":"init"}      + config JSON {"model_path","providers","input_name",
                                     "input_size","classes"}
  <- {"ok":true,...}    or {"ok":false,"error":"..."}
  -> {"op":"infer","n":<bytes>,"name":"images","shape":[1,3,H,W]} + tensor bytes
  <- {"ok":true,"outputs":[{"name":...,"shape":[...]}]}
     + one framed payload per output tensor (little-endian float32)
  -> {"op":"close"}

Requires: onnxruntime, numpy  (see requirements.txt)
"""
import json
import struct
import sys


def read_exact(n):
    buf = b""
    while len(buf) < n:
        chunk = sys.stdin.buffer.read(n - len(buf))
        if not chunk:
            raise EOFError("stdin closed")
        buf += chunk
    return buf


def read_frame():
    (hlen,) = struct.unpack(">I", read_exact(4))
    header = json.loads(read_exact(hlen).decode("utf-8"))
    payload = b""
    n = header.get("n", 0)
    if n:
        payload = read_exact(n)
    return header, payload


def write_frame(header, payload=b""):
    if payload:
        header = dict(header)
        header["n"] = len(payload)
    hb = json.dumps(header).encode("utf-8")
    sys.stdout.buffer.write(struct.pack(">I", len(hb)))
    sys.stdout.buffer.write(hb)
    if payload:
        sys.stdout.buffer.write(payload)
    sys.stdout.buffer.flush()


def fail(msg):
    write_frame({"ok": False, "error": msg})
    sys.exit(1)


def main():
    try:
        import numpy as np
    except Exception as exc:  # pragma: no cover - environment dependent
        fail("worker 依赖缺失：numpy（%s）" % exc)
    try:
        import onnxruntime as ort
    except Exception as exc:  # pragma: no cover - environment dependent
        fail("worker 依赖缺失：onnxruntime（%s）；请 pip install -r requirements.txt" % exc)

    header, payload = read_frame()
    if header.get("op") != "init":
        fail("期望 init 帧，收到 %r" % header.get("op"))
    try:
        cfg = json.loads(payload.decode("utf-8")) if payload else {}
    except Exception as exc:
        fail("init 配置解析失败：%s" % exc)
        return

    model_path = cfg.get("model_path", "")
    providers = cfg.get("providers") or ["CPUExecutionProvider"]
    input_name = cfg.get("input_name", "images")
    if not model_path:
        fail("init 缺少 model_path")
        return
    try:
        so = ort.SessionOptions()
        so.log_severity_level = 3
        session = ort.InferenceSession(model_path, sess_options=so, providers=list(providers))
    except Exception as exc:
        fail("模型加载失败 %s：%s" % (model_path, exc))
        return

    inputs = [{"name": i.name, "shape": i.shape} for i in session.get_inputs()]
    outputs = [{"name": o.name, "shape": o.shape} for o in session.get_outputs()]
    write_frame({"ok": True, "inputs": inputs, "outputs": outputs})

    while True:
        try:
            header, payload = read_frame()
        except EOFError:
            return
        op = header.get("op")
        if op == "close":
            return
        if op != "infer":
            fail("未知操作 %r" % op)
            return
        shape = header.get("shape") or [1, 3, 640, 640]
        name = header.get("name") or input_name
        try:
            tensor = np.frombuffer(payload, dtype="<f4").reshape(
                [int(d) for d in shape]
            )
            results = session.run(None, {name: tensor})
        except Exception as exc:
            write_frame({"ok": False, "error": "推理失败：%s" % exc})
            continue
        meta = [
            {"name": o.name, "shape": [int(d) for d in r.shape]}
            for o, r in zip(session.get_outputs(), results)
        ]
        write_frame({"ok": True, "outputs": meta})
        for r in results:
            data = np.ascontiguousarray(r, dtype="<f4").tobytes()
            sys.stdout.buffer.write(struct.pack(">I", len(data)))
            sys.stdout.buffer.write(data)
        sys.stdout.buffer.flush()


if __name__ == "__main__":
    main()
