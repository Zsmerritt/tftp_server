package main

import (
	"math/rand/v2"
	"ncd/homework/tftp"
	"net"
	"strconv"
	"testing"
	"time"
)

var addressWithoutPort = "localhost:"

func TestServerCreation(t *testing.T) {
	server := setupTestServer(t, 5*time.Second)
	conn := setupServerConnection(t, server)

	time.Sleep(1 * time.Second) // Allow server to start

	// Send a dummy RRQ packet
	rrq := tftp.PacketRequest{
		Op:       tftp.OpRead,
		Filename: "testfile.txt",
		Mode:     "octet",
	}
	sendPacket(t, conn, rrq)

	readAndValidateResponse(t, conn, tftp.OpError)

	server.Close() // Stop the server after the test
	conn.Close()   // Close the connection
}

func TestInvalidRequest(t *testing.T) {
	server := setupTestServer(t, 5*time.Second)
	conn := setupServerConnection(t, server)

	// Send an invalid packet
	_, err := conn.Write([]byte{0x00, 0x00})
	if err != nil {
		t.Fatalf("Failed to send invalid packet: %v", err)
	}

	readAndValidateResponse(t, conn, tftp.OpError)

	server.Close() // Stop the server after the test
	conn.Close()   // Close the connection
}

func TestRRQ(t *testing.T) {
	server := setupTestServer(t, 5*time.Second)
	conn := setupServerConnection(t, server)

	// Add a file to the data store
	server.AddFileToDataStore("testfile.txt", []byte("Hello, World!"))

	// Send an RRQ packet
	rrq := tftp.PacketRequest{
		Op:       tftp.OpRead,
		Filename: "testfile.txt",
		Mode:     "octet",
	}
	sendPacket(t, conn, rrq)

	readAndValidateResponse(t, conn, tftp.OpData)

	server.Close() // Stop the server after the test
	conn.Close()   // Close the connection
}

func TestWRQ(t *testing.T) {
	server := setupTestServer(t, 5*time.Second)
	conn := setupServerConnection(t, server)

	// Send a WRQ packet
	wrq := tftp.PacketRequest{
		Op:       tftp.OpWrite,
		Filename: "newfile.txt",
		Mode:     "octet",
	}
	sendPacket(t, conn, wrq)

	readAndValidateResponse(t, conn, tftp.OpAck)

	server.Close() // Stop the server after the test
	conn.Close()   // Close the connection
}

func TestTimeoutRRQ(t *testing.T) {
	server := setupTestServer(t, 5*time.Second)
	conn := setupServerConnection(t, server)

	lorem := []byte("Lorem ipsum")
	// Add a file to the data store
	server.AddFileToDataStore("Lorem.txt", lorem)

	// Send an RRQ packet
	rrq := tftp.PacketRequest{
		Op:       tftp.OpRead,
		Filename: "Lorem.txt",
		Mode:     "octet",
	}
	sendPacket(t, conn, rrq)
	readAndValidateResponse(t, conn, tftp.OpData)

	// Simulate a timeout
	time.Sleep(6 * time.Second)
	// Check if the server sends request again
	buffer := readAndValidateResponse(t, conn, tftp.OpData)
	var dataPacket tftp.PacketData
	dataPacket.UnmarshalBinary(buffer)

	if string(dataPacket.Data) != string(lorem) {
		t.Fatalf("Expected data to be \n\"%s\",\n got \n\"%s\"", lorem, dataPacket.Data)
	}

	server.Close() // Stop the server after the test
	conn.Close()   // Close the connection
}

func TestWRQ_DATA(t *testing.T) {
	server := setupTestServer(t, 5*time.Second)
	conn := setupServerConnection(t, server)

	// Send a WRQ packet
	wrq := tftp.PacketRequest{
		Op:       tftp.OpWrite,
		Filename: "file.txt",
		Mode:     "octet",
	}

	sendPacket(t, conn, wrq)
	readAndValidateResponse(t, conn, tftp.OpAck)

	// Send a DATA packet
	data := tftp.PacketData{
		Op:       tftp.OpData,
		BlockNum: 1,
		Data:     []byte("1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20"),
	}
	sendPacket(t, conn, data)
	readAndValidateResponse(t, conn, tftp.OpAck)

	server.Close() // Stop the server after the test
	conn.Close()   // Close the connection
}

func TestWRQ_DATA_RRQ(t *testing.T) {
	server := setupTestServer(t, 5*time.Second)
	conn := setupServerConnection(t, server)

	// Send a WRQ packet
	wrq := tftp.PacketRequest{
		Op:       tftp.OpWrite,
		Filename: "file.txt",
		Mode:     "octet",
	}

	sendPacket(t, conn, wrq)
	readAndValidateResponse(t, conn, tftp.OpAck)

	// Send a DATA packet
	data := tftp.PacketData{
		Op:       tftp.OpData,
		BlockNum: 1,
		Data:     []byte("1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20"),
	}
	sendPacket(t, conn, data)
	readAndValidateResponse(t, conn, tftp.OpAck)

	// Send an RRQ packet
	rrq := tftp.PacketRequest{
		Op:       tftp.OpRead,
		Filename: "file.txt",
		Mode:     "octet",
	}
	sendPacket(t, conn, rrq)
	buffer := readAndValidateResponse(t, conn, tftp.OpData)
	var dataPacket tftp.PacketData
	dataPacket.UnmarshalBinary(buffer)
	if string(dataPacket.Data) != string(data.Data) {
		t.Fatalf("Expected data to be \n\"%s\",\n got \n\"%s\"", data.Data, dataPacket.Data)
	}

	server.Close() // Stop the server after the test
	conn.Close()   // Close the connection
}

func TestWRQ_DATA_invalidACK(t *testing.T) {
	server := setupTestServer(t, 5*time.Second)
	conn := setupServerConnection(t, server)

	// Send a WRQ packet
	wrq := tftp.PacketRequest{
		Op:       tftp.OpWrite,
		Filename: "file.txt",
		Mode:     "octet",
	}

	sendPacket(t, conn, wrq)
	readAndValidateResponse(t, conn, tftp.OpAck)

	// Send a DATA packet
	data := tftp.PacketData{
		Op:       tftp.OpData,
		BlockNum: 1,
		Data:     []byte("1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20"),
	}
	sendPacket(t, conn, data)
	readAndValidateResponse(t, conn, tftp.OpAck)

	// Send an ACK packet
	ack := tftp.PacketAck{
		Op:       tftp.OpAck,
		BlockNum: 1,
	}
	sendPacket(t, conn, ack)
	readAndValidateResponse(t, conn, tftp.OpError)

	server.Close() // Stop the server after the test
	conn.Close()   // Close the connection
}

func TestWRQ_DATA_WRQSameFile(t *testing.T) {
	server := setupTestServer(t, 5*time.Second)
	conn := setupServerConnection(t, server)

	// Send a WRQ packet
	wrq := tftp.PacketRequest{
		Op:       tftp.OpWrite,
		Filename: "file.txt",
		Mode:     "octet",
	}

	sendPacket(t, conn, wrq)
	readAndValidateResponse(t, conn, tftp.OpAck)

	// Send a DATA packet
	data := tftp.PacketData{
		Op:       tftp.OpData,
		BlockNum: 1,
		Data:     []byte("1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20"),
	}
	sendPacket(t, conn, data)
	readAndValidateResponse(t, conn, tftp.OpAck)

	// Send another WRQ packet for the same file
	sendPacket(t, conn, wrq)
	readAndValidateResponse(t, conn, tftp.OpError)

	server.Close() // Stop the server after the test
	conn.Close()   // Close the connection
}

func Test_InvalidData(t *testing.T) {
	server := setupTestServer(t, 5*time.Second)
	conn := setupServerConnection(t, server)

	// Send a DATA packet
	data := tftp.PacketData{
		Op:       tftp.OpData,
		BlockNum: 1,
		Data:     []byte("1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20"),
	}
	sendPacket(t, conn, data)
	readAndValidateResponse(t, conn, tftp.OpError)

	server.Close() // Stop the server after the test
	conn.Close()   // Close the connection
}

func TestTwoConnections(t *testing.T) {
	server := setupTestServer(t, 5*time.Second)
	conn1 := setupServerConnection(t, server)
	conn2 := setupServerConnection(t, server)

	lorem := []byte("Lorem ipsum")
	// Add a file to the data store
	server.AddFileToDataStore("Lorem.txt", lorem)

	// Send an RRQ packet
	rrq := tftp.PacketRequest{
		Op:       tftp.OpRead,
		Filename: "Lorem.txt",
		Mode:     "octet",
	}
	sendPacket(t, conn1, rrq)
	sendPacket(t, conn2, rrq)
	readAndValidateResponse(t, conn1, tftp.OpData)
	readAndValidateResponse(t, conn2, tftp.OpData)

	buffer1 := readAndValidateResponse(t, conn1, tftp.OpData)
	var dataPacket1 tftp.PacketData
	dataPacket1.UnmarshalBinary(buffer1)
	buffer2 := readAndValidateResponse(t, conn2, tftp.OpData)
	var dataPacket2 tftp.PacketData
	dataPacket2.UnmarshalBinary(buffer2)

	if string(dataPacket1.Data) != string(lorem) {
		t.Fatalf("Expected data to be \n\"%s\",\n got \n\"%s\"", lorem, dataPacket1.Data)
	}
	if string(dataPacket2.Data) != string(lorem) {
		t.Fatalf("Expected data to be \n\"%s\",\n got \n\"%s\"", lorem, dataPacket2.Data)
	}

	server.Close() // Stop the server after the test
	conn1.Close()  // Close the connection
	conn2.Close()  // Close the connection
}

func setupServerConnection(t *testing.T, server *TFTPServer) *net.UDPConn {
	sftpAddress := server.Address
	port := rand.IntN(65535-1024) + 1024 // Random port between 1024 and 65535
	dialerAddress := addressWithoutPort + strconv.Itoa(port)

	// Create a UDP address
	sftpUdpAddress, err := net.ResolveUDPAddr("udp", sftpAddress)
	if err != nil {
		t.Fatalf("Failed to resolve UDP address: %v", err)
	}

	// Create a UDP address
	dialerUdpAddress, err := net.ResolveUDPAddr("udp", dialerAddress)
	if err != nil {
		t.Fatalf("Failed to resolve UDP address: %v", err)
	}

	// Create a UDP connection
	conn, err := net.DialUDP("udp", dialerUdpAddress, sftpUdpAddress)
	if err != nil {
		t.Fatalf("Failed to dial UDP port: %v", err)
	}

	return conn
}

func setupTestServer(t *testing.T, timeout time.Duration) *TFTPServer {
	port := rand.IntN(65535-1024) + 1024 // Random port between 1024 and 65535
	sftpAddress := addressWithoutPort + strconv.Itoa(port)

	server := NewTFTPServer(sftpAddress, timeout)

	return server
}

func readAndValidateResponse(t *testing.T, conn *net.UDPConn, expectedOp tftp.Op) []byte {
	readResponse, err := readResponse(t, conn)
	if err != nil {
		t.Fatalf("Failed to read from UDP: %v", err)
	}
	validateResponse(t, readResponse, expectedOp)
	return readResponse
}

func readResponse(t *testing.T, conn *net.UDPConn) ([]byte, error) {
	buffer := make([]byte, 516)
	n, _, err := conn.ReadFromUDP(buffer)
	if err != nil {
		return nil, err
	}
	return buffer[:n], nil
}

func validateResponse(t *testing.T, buffer []byte, expectedOp tftp.Op) {
	if len(buffer) < 4 {
		t.Fatalf("Invalid response from server")
	}

	opcode, _ := tftp.PeekOp(buffer[:4])
	if opcode != expectedOp {
		t.Fatalf("Expected opcode %d, got %d", expectedOp, opcode)
	}
}

func sendPacket(
	t *testing.T,
	conn *net.UDPConn,
	packet tftp.Packet) {
	data, err := packet.MarshalBinary()
	if err != nil {
		t.Fatalf("Failed to marshal packet: %v", err)
	}

	_, err = conn.Write(data)
	if err != nil {
		t.Fatalf("Failed to send packet: %v", err)
	}
}
