package protocol

import (
	"bytes"
	"testing"
	"time"
)

func TestBoundedFraming(t *testing.T) {
	var b bytes.Buffer
	in := Request{Version: 1, Network: "tcp", Target: "[::1]:443"}
	if err := Write(&b, in); err != nil {
		t.Fatal(err)
	}
	var out Request
	if err := Read(&b, &out); err != nil || out != in {
		t.Fatalf("roundtrip: %v", err)
	}
	if err := Read(bytes.NewReader([]byte{255, 255, 255, 255}), &out); err == nil {
		t.Fatal("unbounded frame accepted")
	}
}
func TestDatagramsAndMTU(t *testing.T) {
	for _, size := range []int{0, 1, 1200, 1500, 65507} {
		data := bytes.Repeat([]byte{7}, size)
		frames, err := Fragments(3, 8, data)
		if err != nil {
			t.Fatal(err)
		}
		var r Reassembler
		var got []byte
		for i := len(frames) - 1; i >= 0; i-- {
			if len(frames[i]) > 1016 {
				t.Fatal("MTU exceeded")
			}
			id, p, ok, err := r.Push(frames[i], time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				if id != 3 {
					t.Fatal("wrong flow")
				}
				got = p
			}
		}
		if !bytes.Equal(data, got) {
			t.Fatalf("payload mismatch for %d bytes", size)
		}
	}
}
func TestPacketLossAndExpiry(t *testing.T) {
	frames, _ := Fragments(1, 1, make([]byte, 2500))
	var r Reassembler
	now := time.Now()
	if _, _, ok, _ := r.Push(frames[0], now); ok {
		t.Fatal("partial datagram emitted")
	}
	r.Push(frames[2], now)
	if _, _, ok, _ := r.Push(frames[1], now.Add(3*time.Second)); ok {
		t.Fatal("expired assembly emitted")
	}
}
func FuzzFraming(f *testing.F) {
	f.Add([]byte{0, 0, 0, 2, '{', '}'})
	f.Fuzz(func(t *testing.T, b []byte) {
		var r Request
		Read(bytes.NewReader(b), &r)
		var a Reassembler
		a.Push(b, time.Now())
	})
}
