package easytier

// Minimal management RPC adapter for EasyTier v2.6.4. Wire layout is from
// common.proto, rpc_impl/packet.rs and tunnel/packet_def.rs at 8428a89d.
// It deliberately negotiates no compression, opens one bounded call per TCP
// connection, and does not implement the overlay data plane.
import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func wireBytes(b []byte, n protowire.Number, v []byte) []byte {
	b = protowire.AppendTag(b, n, protowire.BytesType)
	return protowire.AppendBytes(b, v)
}
func wireInt(b []byte, n protowire.Number, v uint64) []byte {
	b = protowire.AppendTag(b, n, protowire.VarintType)
	return protowire.AppendVarint(b, v)
}

type wireField struct {
	number protowire.Number
	bytes  []byte
	value  uint64
}

func wireFields(b []byte) ([]wireField, error) {
	var fields []wireField
	for len(b) > 0 {
		n, t, k := protowire.ConsumeTag(b)
		if k < 0 {
			return nil, protowire.ParseError(k)
		}
		b = b[k:]
		f := wireField{number: n}
		switch t {
		case protowire.BytesType:
			f.bytes, k = protowire.ConsumeBytes(b)
		case protowire.VarintType:
			f.value, k = protowire.ConsumeVarint(b)
		default:
			k = protowire.ConsumeFieldValue(n, t, b)
		}
		if k < 0 {
			return nil, protowire.ParseError(k)
		}
		b = b[k:]
		fields = append(fields, f)
	}
	return fields, nil
}

func managementRPC(ctx context.Context, address, service string, method uint64, request []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	desc := wireBytes(nil, 2, []byte(service))
	desc = wireBytes(desc, 3, []byte(service))
	desc = wireInt(desc, 4, method)
	body := wireBytes(nil, 2, request)
	body = wireInt(body, 3, 7000)
	if len(body) > 1024*1024 {
		return nil, fmt.Errorf("RPC request exceeds 1 MiB limit")
	}
	count := (len(body) + 899) / 900
	for index := 0; index < count; index++ {
		packet := wireInt(nil, 1, 1)
		packet = wireInt(packet, 2, 1)
		packet = wireInt(packet, 3, 1)
		packet = wireBytes(packet, 4, desc)
		packet = wireBytes(packet, 5, body[index*900:min((index+1)*900, len(body))])
		packet = wireInt(packet, 6, 1)
		packet = wireInt(packet, 7, uint64(count))
		packet = wireInt(packet, 8, uint64(index))
		compression := wireInt(nil, 1, 1)
		compression = wireInt(compression, 2, 1)
		packet = wireBytes(packet, 10, compression)
		frame := make([]byte, 20)
		binary.LittleEndian.PutUint32(frame, uint32(16+len(packet)))
		binary.LittleEndian.PutUint32(frame[4:], 1)
		binary.LittleEndian.PutUint32(frame[8:], 1)
		frame[12] = 8
		binary.LittleEndian.PutUint32(frame[16:], uint32(len(packet)))
		frame = append(frame, packet...)
		if _, err = io.Copy(conn, bytes.NewReader(frame)); err != nil {
			return nil, err
		}
	}
	var frame []byte
	pieces := map[uint64][]byte{}
	var total uint64
	for {
		var header [4]byte
		if _, err = io.ReadFull(conn, header[:]); err != nil {
			return nil, err
		}
		size := binary.LittleEndian.Uint32(header[:])
		if size < 16 || size > 2000 {
			return nil, fmt.Errorf("invalid RPC frame size %d", size)
		}
		frame = make([]byte, size)
		if _, err = io.ReadFull(conn, frame); err != nil {
			return nil, err
		}
		if frame[8] != 9 {
			return nil, fmt.Errorf("unexpected RPC packet type %d", frame[8])
		}
		fields, err := wireFields(frame[16:])
		if err != nil {
			return nil, err
		}
		var index, count uint64
		var data []byte
		for _, f := range fields {
			switch f.number {
			case 3:
				if f.value != 1 {
					return nil, fmt.Errorf("RPC transaction mismatch")
				}
			case 5:
				data = f.bytes
			case 7:
				count = f.value
			case 8:
				index = f.value
			case 10:
				ci, e := wireFields(f.bytes)
				if e != nil {
					return nil, e
				}
				for _, v := range ci {
					if v.number == 1 && v.value != 1 {
						return nil, fmt.Errorf("RPC compression was not negotiated")
					}
				}
			}
		}
		if count == 0 && index == 0 {
			count = 1
		}
		if count == 0 || count > 4096 || index >= count || (total != 0 && total != count) {
			return nil, fmt.Errorf("invalid RPC fragment")
		}
		if _, exists := pieces[index]; exists {
			return nil, fmt.Errorf("duplicate RPC fragment")
		}
		total = count
		pieces[index] = append([]byte(nil), data...)
		if uint64(len(pieces)) != total {
			continue
		}
		var response []byte
		for i := uint64(0); i < total; i++ {
			response = append(response, pieces[i]...)
		}
		envelope, err := wireFields(response)
		if err != nil {
			return nil, err
		}
		var result []byte
		for _, f := range envelope {
			if f.number == 2 && len(f.bytes) > 0 {
				return nil, fmt.Errorf("Core rejected %s method %d: %s", service, method, rpcErrorDetail(f.bytes))
			}
			if f.number == 1 {
				result = f.bytes
			}
		}
		return result, nil
	}
}
