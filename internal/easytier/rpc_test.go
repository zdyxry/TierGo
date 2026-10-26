package easytier

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestDecodeCoreEvent(t *testing.T) {
	event, err := decodeCoreEvent([]byte(`{"time":"2026-10-22T12:00:00+08:00","event":{"ListenerAdded":"tcp://0.0.0.0:11010"}}`))
	if err != nil || event.Kind != "ListenerAdded" || event.Source != "Core 事件" {
		t.Fatalf("event=%+v error=%v", event, err)
	}
	if _, err := decodeCoreEvent([]byte(`{"time":"broken"}`)); err == nil {
		t.Fatal("invalid timestamp accepted")
	}
}
func TestWireFieldsRejectMalformed(t *testing.T) {
	if _, err := wireFields([]byte{0xff}); err == nil {
		t.Fatal("invalid protobuf accepted")
	}
}

// Exercise fragmentation and hostile frame lengths with a real TCP peer.
func TestRPCFragmentedResponse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var head [4]byte
		if _, err = io.ReadFull(conn, head[:]); err != nil {
			return
		}
		request := make([]byte, binary.LittleEndian.Uint32(head[:]))
		if _, err = io.ReadFull(conn, request); err != nil {
			return
		}
		response := wireBytes(nil, 1, []byte("payload"))
		for index, part := range [][]byte{response[:3], response[3:]} {
			packet := wireInt(nil, 3, 1)
			packet = wireBytes(packet, 5, part)
			packet = wireInt(packet, 7, 2)
			packet = wireInt(packet, 8, uint64(index))
			frame := make([]byte, 20)
			binary.LittleEndian.PutUint32(frame, uint32(16+len(packet)))
			frame[12] = 9
			_, _ = conn.Write(append(frame, packet...))
		}
	}()
	result, err := managementRPC(context.Background(), listener.Addr().String(), "test", 1, nil)
	if err != nil || string(result) != "payload" {
		t.Fatalf("response=%q err=%v", result, err)
	}
}

func TestE2ECollectNativeEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("real Core integration")
	}
	ctl, _ := testController(t)
	ctl.engine = nativeEngine{}
	ctl.Start(testConfig())
	snap := waitForPhase(t, ctl, PhaseRunning, 30*time.Second)
	events, err := collectCoreEvents(context.Background(), snap.Portal)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind == "ListenerAdded" {
			return
		}
	}
	t.Fatalf("Core listener event missing: %+v", events)
}
