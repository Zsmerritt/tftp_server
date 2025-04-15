package tftp

import (
	"encoding"
	"reflect"
	"testing"
)

func TestSerializationDeserialization(t *testing.T) {
	tests := []struct {
		bytes  []byte
		packet interface {
			MarshalBinary() ([]byte, error)
			UnmarshalBinary([]byte) error
		}
	}{
		{
			[]byte("\x00\x01foo\x00bar\x00"),
			&PacketRequest{OpRead, "foo", "bar"},
		},
		{
			[]byte("\x00\x02foo\x00bar\x00"),
			&PacketRequest{OpWrite, "foo", "bar"},
		},
		{
			[]byte("\x00\x03\x12\x34fnord"),
			&PacketData{OpData, 0x1234, []byte("fnord")},
		},
		{
			[]byte("\x00\x03\x12\x34"),
			&PacketData{OpData, 0x1234, []byte("")},
		},
		{
			[]byte("\x00\x04\xd0\x0f"),
			&PacketAck{OpAck, 0xd00f},
		},
		{
			[]byte("\x00\x05\xab\xcdparachute failure\x00"),
			&PacketError{OpError, 0xabcd, "parachute failure"},
		},
	}

	for _, test := range tests {
		actualBytes, _ := test.packet.MarshalBinary()
		if !reflect.DeepEqual(test.bytes, actualBytes) {
			t.Errorf("Serializing %#v: expected %q; got %q", test.packet, test.bytes, actualBytes)
		}

		rt := reflect.TypeOf(test.packet).Elem()
		np := reflect.New(rt).Interface().(encoding.BinaryUnmarshaler)
		err := np.UnmarshalBinary(test.bytes)
		if err != nil {
			t.Errorf("Unable to parse packet %q: %s", test.bytes, err)
		} else if !reflect.DeepEqual(test.packet, np) {
			t.Errorf("Deserializing %q: expected %#v; got %#v", test.bytes, test.packet, np)
		}
	}
}
