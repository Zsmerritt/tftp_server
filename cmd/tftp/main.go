package main

import (
	"log"
	"ncd/homework/tftp"
	"net"
	"sync"
	"time"
)

func main() {
	address := "localhost:69"
	timeout := 5 * time.Second

	NewTFTPServer(address, timeout)
}

// // TFTPServer represents a simple TFTP server.
type TFTPServer struct {
	Address                string
	Timeout                time.Duration
	dataStore              map[string]map[uint16][]byte // Filename -> BlockNum -> Data
	dataStoreMutex         sync.RWMutex
	incomingFiles          map[string]map[uint16][]byte // Addr -> BlockNum -> Data
	incomingFilesMutex     sync.RWMutex
	incomingFileNames      map[string]string // Addr -> Filename
	incomingFileNamesMutex sync.RWMutex
	unAckedPackets         map[string]map[uint16][]byte // Addr -> BlockNum -> Data
	unAckedPacketsMutex    sync.RWMutex
	outgoingFiles          map[string]string // Addr -> Filename
	outgoingFilesMutex     sync.RWMutex
	buffer                 []byte
	conn                   *net.UDPConn
}

// // NewTFTPServer creates a new TFTP server instance.
func NewTFTPServer(address string, timeout time.Duration) *TFTPServer {
	// Create a UDP address
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}

	// Create a UDP connection
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on UDP port: %v", err)
	}
	log.Printf("[TFTP] TFTP server listening on %s", address)

	var instance = &TFTPServer{
		Address:           address,
		Timeout:           timeout,
		dataStore:         make(map[string]map[uint16][]byte), // Filename -> BlockNum -> Data
		incomingFiles:     make(map[string]map[uint16][]byte), // Addr -> BlockNum -> Data
		incomingFileNames: make(map[string]string),            // Addr -> Filename
		unAckedPackets:    make(map[string]map[uint16][]byte), // Addr -> BlockNum -> Data
		outgoingFiles:     make(map[string]string),            // Addr -> Filename
		buffer:            make([]byte, 516),                  // TFTP packets are typically 512 bytes + header
		conn:              conn,
	}

	go instance.listen()
	return instance
}

// // Close closes the UDP connection of the TFTP server.
func (s *TFTPServer) Close() {
	s.conn.Close()
}

// // AddFileToDataStore adds a file to the data store for testing purposes.
func (s *TFTPServer) AddFileToDataStore(filename string, data []byte) {
	s.dataStoreMutex.Lock()

	// Check if the file already exists
	_, exists := s.dataStore[filename]
	if !exists {
		s.dataStore[filename] = make(map[uint16][]byte)
	}

	// break the data into blocks of 512 bytes and add them to the data store
	for i := 0; i < len(data); i += 512 {
		blockNum := uint16((i / 512) + 1) // Block numbers start at 1
		end := i + 512
		if end > len(data) {
			end = len(data)
		}
		s.dataStore[filename][blockNum] = data[i:end]
	}

	s.dataStoreMutex.Unlock()
}

// // listen starts listening for incoming TFTP requests on the UDP connection.
func (s *TFTPServer) listen() {
	defer s.conn.Close()

	for {
		// Read from the connection
		n, clientAddr, err := s.conn.ReadFromUDP(s.buffer)
		if err != nil {
			// log.Printf("[TFTP] Error reading from UDP: %v", err)
			continue
		}

		log.Printf("[TFTP] Received %d bytes from %s", n, clientAddr)

		// Handle the request in a separate goroutine
		go s.handleRequest(clientAddr, s.buffer[:n])
	}
}

// // handleRequest processes incoming TFTP requests
// // It parses the request, checks the operation type (RRQ, WRQ, DATA, ACK), and responds accordingly.
// // It also manages the data store and handles file transfers.
func (s *TFTPServer) handleRequest(clientAddr *net.UDPAddr, data []byte) {
	// Parse the incoming request (e.g., RRQ, WRQ, DATA, ACK)
	if len(data) < 4 {
		log.Printf("[TFTP] Invalid TFTP request from %s", clientAddr)
		s.sendErrorPacket(clientAddr, tftp.ErrIllegal)
		return
	}

	opcode, _ := tftp.PeekOp(data) // Extract opcode
	switch opcode {
	case tftp.OpRead: // RRQ (Read Request)
		log.Printf("[TFTP] Received RRQ from %s", clientAddr)
		var readPacket tftp.PacketRequest
		readPacket.UnmarshalBinary(data)
		err := readPacket.UnmarshalBinary(data)
		if err != nil {
			log.Printf("[TFTP] Failed to parse RRQ packet from %s: %v", clientAddr, err)
			s.sendErrorPacket(clientAddr, tftp.ErrIllegal)
			return
		}

		// Check if that file exists and is complete
		s.dataStoreMutex.RLock()
		storedData, ok := s.dataStore[readPacket.Filename][1]
		s.dataStoreMutex.RUnlock()

		if ok {
			// Respond with a DATA packet
			s.sendDataPacketWithTimeout(clientAddr, uint16(1) /* BlockNum */, storedData)
		} else {
			// Respond with an ERROR packet
			s.sendErrorPacket(clientAddr, tftp.ErrFileNotFound)
		}

		// Store the file name in the outgoing files map
		s.outgoingFilesMutex.Lock()
		s.outgoingFiles[clientAddr.String()] = readPacket.Filename
		s.outgoingFilesMutex.Unlock()

		return
	case tftp.OpWrite: // WRQ (Write Request)
		log.Printf("[TFTP] Received WRQ from %s", clientAddr)
		var writePacket tftp.PacketRequest
		err := writePacket.UnmarshalBinary(data)
		if err != nil {
			log.Printf("[TFTP] Failed to parse WRQ packet from %s: %v", clientAddr, err)
			s.sendErrorPacket(clientAddr, tftp.ErrIllegal)
			return
		}

		// Check if the file already exists
		s.dataStoreMutex.RLock()
		_, exists := s.dataStore[writePacket.Filename]
		s.dataStoreMutex.RUnlock()
		if exists {
			// Respond with an ERROR packet
			s.sendErrorPacket(clientAddr, tftp.ErrExists)
			return
		}

		// Initialize the data store for the file
		s.incomingFilesMutex.Lock()
		s.incomingFiles[clientAddr.String()] = map[uint16][]byte{}
		s.incomingFilesMutex.Unlock()

		s.incomingFileNamesMutex.Lock()
		s.incomingFileNames[clientAddr.String()] = writePacket.Filename
		s.incomingFileNamesMutex.Unlock()

		// Respond with an ACK packet
		s.sendAckPacketWithTimeout(clientAddr, 0 /* BlockNum */)
		return

	case tftp.OpData: // DATA (Data Packet)
		var dataPacket tftp.PacketData
		err := dataPacket.UnmarshalBinary(data)
		if err != nil {
			log.Printf("[TFTP] Failed to parse DATA packet from %s: %v", clientAddr, err)
			s.sendErrorPacket(clientAddr, tftp.ErrIllegal)
			return
		}
		log.Printf("[TFTP] DATA Packet: BlockNum=%d, Data=%s", dataPacket.BlockNum, string(dataPacket.Data))

		// Packet acknowledged, remove from unacknowledged packets
		s.unAckedPacketsMutex.Lock()
		// Subtract 1 as recieving this packet indicates the previous one was received
		delete(s.unAckedPackets[clientAddr.String()], dataPacket.BlockNum-1)
		s.unAckedPacketsMutex.Unlock()

		// Store the received data in the data store
		s.incomingFilesMutex.Lock()
		if _, exists := s.incomingFiles[clientAddr.String()]; !exists {
			log.Printf("[TFTP] No WRQ found for client %s", clientAddr)
			s.incomingFilesMutex.Unlock()
			s.sendErrorPacket(clientAddr, tftp.ErrAccessViolation)
			return
		}
		dataCopy := make([]byte, len(dataPacket.Data))
		copy(dataCopy, dataPacket.Data)
		s.incomingFiles[clientAddr.String()][dataPacket.BlockNum] = dataCopy
		s.incomingFilesMutex.Unlock()

		// Check if this is the last block
		if len(dataPacket.Data) < 512 {
			s.moveCompletedFileToStore(clientAddr.String())
			log.Printf("[TFTP] File transfer complete for %s", clientAddr.String())
			s.sendAckPacket(clientAddr, dataPacket.BlockNum)
			return
		}
		// Respond with an ACK packet
		s.sendAckPacketWithTimeout(clientAddr, dataPacket.BlockNum)
		return

	case tftp.OpAck: // ACK (Acknowledgment Packet)
		var ackPacket tftp.PacketAck
		err := ackPacket.UnmarshalBinary(data)
		if err != nil {
			log.Printf("[TFTP] Failed to parse ACK packet from %s: %v", clientAddr, err)
			s.sendErrorPacket(clientAddr, tftp.ErrIllegal)
			return
		}
		log.Printf("[TFTP] Received ACK (block %d) from %s", ackPacket.BlockNum, clientAddr.String())

		// Packet acknowledged, remove from unacknowledged packets
		s.unAckedPacketsMutex.Lock()
		delete(s.unAckedPackets[clientAddr.String()], ackPacket.BlockNum)
		s.unAckedPacketsMutex.Unlock()

		// Get the file name from the outgoing files map
		s.outgoingFilesMutex.RLock()
		filename, ok := s.outgoingFiles[clientAddr.String()]
		s.outgoingFilesMutex.RUnlock()
		if !ok {
			log.Printf("[TFTP] No outgoing file for client %s", clientAddr)
			// Respond with an ERROR packet
			s.sendErrorPacket(clientAddr, tftp.ErrAccessViolation)
			return
		}

		s.dataStoreMutex.RLock()
		payload, ok := s.dataStore[filename][ackPacket.BlockNum+1]
		s.dataStoreMutex.RUnlock()
		if !ok {
			// No data for the next block, transfer is complete
			log.Printf("[TFTP] Transfer is complete for client %s", clientAddr.String())

			// Remove the filename from the outgoing files map
			s.outgoingFilesMutex.Lock()
			delete(s.outgoingFiles, clientAddr.String())
			s.outgoingFilesMutex.Unlock()
			return
		}
		// Add one to the block number to indicate the next block
		s.sendDataPacketWithTimeout(clientAddr, ackPacket.BlockNum+1, payload)
		return

	default:
		log.Printf("[TFTP] Unsupported opcode %d from %s", opcode, clientAddr)
		// Respond with an ERROR packet
		s.sendErrorPacket(clientAddr, tftp.ErrIllegal)
	}
}

// // sendDataPacketWithTimeout sends a DATA packet to the client and starts a timeout for unacknowledged packets.
func (s *TFTPServer) sendDataPacketWithTimeout(clientAddr *net.UDPAddr, blockNum uint16, data []byte) {
	s.sendDataPacket(clientAddr, blockNum, data)
	// Start timeout for the unacknowledged packet
	go s.timeoutWithResend(clientAddr, blockNum)
}

// // sendDataPacket sends a DATA packet to the client.
func (s *TFTPServer) sendDataPacket(clientAddr *net.UDPAddr, blockNum uint16, data []byte) {
	packetData := tftp.PacketData{
		Op:       tftp.OpData,
		BlockNum: blockNum,
		Data:     data,
	}
	packetDataBytes, err := packetData.MarshalBinary()

	s.unAckedPacketsMutex.Lock()
	_, ok := s.unAckedPackets[clientAddr.String()]
	if !ok {
		s.unAckedPackets[clientAddr.String()] = make(map[uint16][]byte)
	}
	s.unAckedPackets[clientAddr.String()][blockNum] = packetDataBytes
	s.unAckedPacketsMutex.Unlock()

	if err != nil {
		log.Printf("[TFTP] Error marshalling DATA packet: %v", err)
		return
	} else {
		_, err = s.conn.WriteToUDP(packetDataBytes, clientAddr)
		if err != nil {
			log.Printf("[TFTP] Error sending DATA to %s: %v", clientAddr, err)
			return
		}
		log.Printf("[TFTP] Sent DATA to %s: BlockNum: %d", clientAddr, blockNum)
	}
}

// // sendAckPacketWithTimeout sends an ACK packet to the client and starts a timeout for unacknowledged packets.
func (s *TFTPServer) sendAckPacketWithTimeout(clientAddr *net.UDPAddr, blockNum uint16) {
	s.sendAckPacket(clientAddr, blockNum)
	// Start timeout for the unacknowledged packet
	go s.timeoutWithResend(clientAddr, blockNum)
}

// // sendAckPacket sends an ACK packet to the client.
func (s *TFTPServer) sendAckPacket(clientAddr *net.UDPAddr, blockNum uint16) {
	packetAck := tftp.PacketAck{
		Op:       tftp.OpAck,
		BlockNum: blockNum,
	}
	packetAckBytes, err := packetAck.MarshalBinary()

	s.unAckedPacketsMutex.Lock()
	_, ok := s.unAckedPackets[clientAddr.String()]
	if !ok {
		s.unAckedPackets[clientAddr.String()] = make(map[uint16][]byte)
	}
	s.unAckedPackets[clientAddr.String()][blockNum] = packetAckBytes
	s.unAckedPacketsMutex.Unlock()

	if err != nil {
		log.Printf("[TFTP] Error marshalling ACK packet: %v", err)
		return
	} else {
		_, err = s.conn.WriteToUDP(packetAckBytes, clientAddr)
		if err != nil {
			log.Printf("[TFTP] Error sending ACK to %s: %v", clientAddr, err)
			return
		}
		log.Printf("[TFTP] Sent ACK to %s: BlockNum: %d", clientAddr, blockNum)
	}
}

// // sendErrorPacket sends an ERROR packet to the client.
func (s *TFTPServer) sendErrorPacket(clientAddr *net.UDPAddr, errorCode tftp.ErrorCode) {
	packetError := tftp.PacketError{
		Op:    tftp.OpError,
		Error: errorCode,
		Msg:   errorCode.Error(),
	}
	packetErrorBytes, err := packetError.MarshalBinary()
	if err != nil {
		log.Printf("[TFTP] Error marshalling ERROR packet: %v", err)
		return
	} else {
		_, err = s.conn.WriteToUDP(packetErrorBytes, clientAddr)
		if err != nil {
			log.Printf("[TFTP] Error sending ERROR to %s: %v", clientAddr, err)
			return
		}
		log.Printf("[TFTP] Sent ERROR to %s: %s", clientAddr, errorCode.Error())
	}
}

// // timeoutWithResend checks for unacknowledged packets and resends them if necessary.
func (s *TFTPServer) timeoutWithResend(clientAddr *net.UDPAddr, blockNum uint16) {
	time.Sleep(s.Timeout)
	s.unAckedPacketsMutex.RLock()
	data, ok := s.unAckedPackets[clientAddr.String()][blockNum]
	if ok {
		log.Printf("[TFTP] Timeout for block %d from %s, resending", blockNum, clientAddr)
		_, err := s.conn.WriteToUDP(data, clientAddr)
		if err != nil {
			log.Printf("[TFTP] Error resending DATA to %s: %v", clientAddr, err)
		}
	}
	s.unAckedPacketsMutex.RUnlock()
}

// // moveCompletedFileToStore moves the completed file from incomingFiles to dataStore.
func (s *TFTPServer) moveCompletedFileToStore(clientAddrString string) {
	// Get and delete the completed file from incomingFiles
	s.incomingFilesMutex.Lock()
	finishedFile, exists := s.incomingFiles[clientAddrString]
	if !exists {
		log.Printf("[TFTP] No completed file found for client %s", clientAddrString)
		s.incomingFilesMutex.Unlock()
		return
	}
	delete(s.incomingFiles, clientAddrString)
	s.incomingFilesMutex.Unlock()

	// Get and delete the file name from incomingFileNames
	s.incomingFileNamesMutex.Lock()
	fileName, exists := s.incomingFileNames[clientAddrString]
	if !exists {
		log.Printf("[TFTP] No file name found for client %s", clientAddrString)
		s.incomingFileNamesMutex.Unlock()
		return
	}
	delete(s.incomingFileNames, clientAddrString)
	s.incomingFileNamesMutex.Unlock()

	// Move the completed file to the dataStore
	s.dataStoreMutex.Lock()
	s.dataStore[fileName] = finishedFile
	s.dataStoreMutex.Unlock()

	log.Printf("[TFTP] File %s successfully moved to dataStore", fileName)
}
