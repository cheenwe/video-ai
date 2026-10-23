package videoai

import (
	"encoding/binary"
	"io"
	"math"
	"net"
	"testing"
	"time"
)

// fakeWorker speaks the worker protocol on its end of the pipe. The script
// func receives each request header+payload and returns the reply header and
// output payloads.
func fakeWorker(t *testing.T, rw io.ReadWriteCloser, script func(hdr map[string]any, payload []byte) (map[string]any, [][]byte)) {
	t.Helper()
	go func() {
		for {
			hdr, payload, err := readFrame(rw)
			if err != nil {
				return
			}
			reply, outs := script(hdr, payload)
			if err := writeFrame(rw, reply, nil); err != nil {
				return
			}
			for _, o := range outs {
				if err := writeFrame(rw, map[string]any{"n": len(o)}, o); err != nil {
					return
				}
			}
		}
	}()
}

func testSessionCfg() ModelConfig {
	return ModelConfig{
		Adapter: "onnx", ModelPath: "/tmp/fake.onnx",
		InputW: 640, InputH: 640, Classes: []string{"grab_bucket"},
	}
}

// TestWorkerProtocolRoundTrip covers init handshake + one inference through
// the real framing code with an in-process fake worker.
func TestWorkerProtocolRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	var gotShape []int
	fakeWorker(t, server, func(hdr map[string]any, payload []byte) (map[string]any, [][]byte) {
		switch hdr["op"] {
		case "init":
			return map[string]any{"ok": true, "outputs": []map[string]any{{"name": "output0", "shape": []int{1, 5, 3}}}}, nil
		case "infer":
			gotShape = intSlice(hdr["shape"])
			if len(payload) != 3*640*640*4 {
				return map[string]any{"ok": false, "error": "tensor size mismatch"}, nil
			}
			// One detection: cx=100, cy=100, 40x40, score 0.9 (channel-first).
			data := channelFirst(1, 1, [][]float32{{100, 100, 40, 40, 0.9}})
			raw := make([]byte, len(data)*4)
			for i, v := range data {
				binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(v))
			}
			return map[string]any{"ok": true, "outputs": []map[string]any{{"name": "output0", "shape": []int{1, 5, 1}}}}, [][]byte{raw}
		}
		return map[string]any{"ok": false, "error": "unexpected op"}, nil
	})

	s := newWorkerSessionWith(testSessionCfg(), client)
	defer s.close()
	if err := s.handshake(); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	tensor := make([]float32, 3*640*640)
	outs, err := s.run(tensor, testSessionCfg())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(outs) != 1 || outs[0].Name != "output0" {
		t.Fatalf("outputs %+v", outs)
	}
	if len(gotShape) != 4 || gotShape[2] != 640 {
		t.Fatalf("worker saw shape %v", gotShape)
	}
	dets, err := postprocess(outs, testSessionCfg(), letterbox{gain: 1, inW: 640, inH: 640}, 640, 640)
	if err != nil {
		t.Fatalf("postprocess: %v", err)
	}
	if len(dets) != 1 || dets[0].Centroid.X != 100 {
		t.Fatalf("detections %+v", dets)
	}
}

// TestWorkerProtocolErrorMarksDead: an error reply fails the call and kills
// the session so the next tick respawns.
func TestWorkerProtocolErrorMarksDead(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	fakeWorker(t, server, func(hdr map[string]any, payload []byte) (map[string]any, [][]byte) {
		if hdr["op"] == "init" {
			return map[string]any{"ok": true}, nil
		}
		return map[string]any{"ok": false, "error": "model exploded"}, nil
	})
	s := newWorkerSessionWith(testSessionCfg(), client)
	defer s.close()
	if err := s.handshake(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.run(make([]float32, 4), testSessionCfg()); err == nil {
		t.Fatal("expected inference error")
	}
	if s.alive() {
		t.Fatal("session must be marked dead after a failed call")
	}
	if _, err := s.run(make([]float32, 4), testSessionCfg()); err == nil {
		t.Fatal("dead session must refuse further calls")
	}
}

// TestWorkerProtocolTimeoutKillsHungWorker: a silent worker must not stall
// the analysis loop forever.
func TestWorkerProtocolTimeoutKillsHungWorker(t *testing.T) {
	old := inferTimeout
	inferTimeout = 150 * time.Millisecond
	defer func() { inferTimeout = old }()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	// The fake answers init but then goes silent.
	fakeWorker(t, server, func(hdr map[string]any, payload []byte) (map[string]any, [][]byte) {
		if hdr["op"] == "init" {
			return map[string]any{"ok": true}, nil
		}
		time.Sleep(2 * time.Second)
		return map[string]any{"ok": true}, nil
	})
	s := newWorkerSessionWith(testSessionCfg(), client)
	defer s.close()
	if err := s.handshake(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := s.run(make([]float32, 4), testSessionCfg()); err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timeout took too long: %v", time.Since(start))
	}
	if s.alive() {
		t.Fatal("timed-out session must be dead")
	}
}

// TestFramingRejectsOversizedHeader keeps a corrupt stream from allocating
// unbounded memory.
func TestFramingRejectsOversizedHeader(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, 1<<21)
		_, _ = server.Write(buf)
	}()
	if _, _, err := readFrame(client); err == nil {
		t.Fatal("expected oversized header rejection")
	}
}
