package tftp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// larger than a typical mtu (1500), and largest DATA packet (516).
// may limit the length of filenames in RRQ/WRQs -- RFC1350 doesn't offer a bound for these.
const MaxPacketSize = 2048

//go:generate stringer -type=Op
type Op uint16

const (
	OpRead  Op = 1
	OpWrite Op = 2
	OpData  Op = 3
	OpAck   Op = 4
	OpError Op = 5
)

// PeekOp determines the operation type of a TFTP packet
func PeekOp(b []byte) (Op, error) {
	if len(b) < 2 {
		return 0, io.ErrShortBuffer
	}
	return Op(binary.BigEndian.Uint16(b[:2])), nil
}

type ErrorCode uint16

const (
	ErrNotDefined      ErrorCode = 1
	ErrFileNotFound    ErrorCode = 2
	ErrAccessViolation ErrorCode = 3
	ErrDiskFull        ErrorCode = 4
	ErrIllegal         ErrorCode = 5
	ErrExists          ErrorCode = 6
	ErrUnknownUser     ErrorCode = 7
)

var errorStrings = map[ErrorCode]string{
	ErrNotDefined:      "Not defined",
	ErrFileNotFound:    "File not found",
	ErrAccessViolation: "Access Violation",
	ErrDiskFull:        "Disk full or allocation exceeded",
	ErrIllegal:         "Illegal TFTP operation",
	ErrExists:          "File already exists",
	ErrUnknownUser:     "No such user",
}

func (e ErrorCode) Error() string {
	if e == 0 {
		return ""
	}
	if s, ok := errorStrings[e]; ok {
		return fmt.Sprintf("TFTP Error(%d) - %s", e, s)
	}
	return fmt.Sprintf("TFTP Error(%d) - Unknown", e)
}

type Packet interface {
	MarshalBinary() ([]byte, error)
}

// PacketRequest represents a request to read from or write to a file.
type PacketRequest struct {
	Op       Op
	Filename string
	Mode     string
}

func (p PacketRequest) MarshalBinary() ([]byte, error) {
	var b []byte
	b = binary.BigEndian.AppendUint16(b, uint16(p.Op))
	b = append(b, p.Filename...)
	b = append(b, 0)
	b = append(b, p.Mode...)
	b = append(b, 0)
	return b, nil
}

func (p *PacketRequest) UnmarshalBinary(b []byte) error {
	d := decoder{p: b}
	p.Op = Op(d.uint16())
	p.Filename = d.string()
	p.Mode = d.string()
	return d.err
}

// PacketData carries a block of data in a file transmission.
type PacketData struct {
	Op       Op
	BlockNum uint16
	Data     []byte
}

func (p PacketData) MarshalBinary() ([]byte, error) {
	var b []byte
	b = binary.BigEndian.AppendUint16(b, uint16(p.Op))
	b = binary.BigEndian.AppendUint16(b, p.BlockNum)
	b = append(b, p.Data...)
	return b, nil
}

func (p *PacketData) UnmarshalBinary(b []byte) error {
	d := decoder{p: b}
	p.Op = Op(d.uint16())
	p.BlockNum = d.uint16()
	p.Data = d.data()
	return d.err
}

// PacketAck acknowledges receipt of a data packet
type PacketAck struct {
	Op       Op
	BlockNum uint16
}

func (p PacketAck) MarshalBinary() ([]byte, error) {
	var b []byte
	b = binary.BigEndian.AppendUint16(b, uint16(p.Op))
	b = binary.BigEndian.AppendUint16(b, p.BlockNum)
	return b, nil
}

func (p *PacketAck) UnmarshalBinary(b []byte) error {
	d := decoder{p: b}
	p.Op = Op(d.uint16())
	p.BlockNum = d.uint16()
	return d.err
}

// PacketError is sent by a peer who has encountered an error condition
type PacketError struct {
	Op    Op
	Error ErrorCode
	Msg   string
}

func (p PacketError) MarshalBinary() ([]byte, error) {
	var b []byte
	b = binary.BigEndian.AppendUint16(b, uint16(p.Op))
	b = binary.BigEndian.AppendUint16(b, uint16(p.Error))
	b = append(b, p.Msg...)
	b = append(b, 0)
	return b, nil
}

func (p *PacketError) UnmarshalBinary(b []byte) error {
	d := decoder{p: b}
	p.Op = Op(d.uint16())
	p.Error = ErrorCode(d.uint16())
	p.Msg = d.string()
	return d.err
}

type decoder struct {
	p   []byte
	err error
}

func (d *decoder) uint16() uint16 {
	if d.err != nil {
		return 0
	}
	if len(d.p) < 2 {
		d.err = io.ErrShortBuffer
		return 0
	}
	v := binary.BigEndian.Uint16(d.p)
	d.p = d.p[2:]
	return v
}

func (d *decoder) string() string {
	if d.err != nil {
		return ""
	}
	s, p, ok := bytes.Cut(d.p, []byte{0})
	if !ok {
		d.err = io.ErrUnexpectedEOF
		return ""
	}
	d.p = p
	return string(s)
}

func (d *decoder) data() []byte {
	if d.err != nil {
		return nil
	}
	return d.p
}
