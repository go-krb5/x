package ndr

import "encoding/binary"

func (enc *Encoder) writeCommonHeader() error {
	enc.ch.Version = protocolVersion
	enc.ch.HeaderLength = commonHeaderBytes
	enc.ch.CharacterEncoding = ascii
	enc.ch.FloatRepresentation = ieee
	representation := byte(littleEndian)
	if enc.ch.Endianness == binary.ByteOrder(binary.BigEndian) {
		representation = byte(bigEndian)
	}
	// Version
	if err := enc.buf.WriteByte(protocolVersion); err != nil {
		return Errorf("could not write common header version: %v", err)
	}
	// Endianness (high nibble) and character encoding (low nibble)
	if err := enc.buf.WriteByte(representation<<4 | enc.ch.CharacterEncoding); err != nil {
		return Errorf("could not write common header endianness: %v", err)
	}
	// Common header length
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, commonHeaderBytes)
	if err := enc.writeBytes(b); err != nil {
		return Errorf("could not write common header length: %v", err)
	}
	// Filler bytes: MUST be set to 0xcccccccc on marshaling.
	enc.ch.Filler = []byte{0xcc, 0xcc, 0xcc, 0xcc}
	if err := enc.writeBytes(enc.ch.Filler); err != nil {
		return Errorf("could not write common header filler: %v", err)
	}
	return nil
}

func (enc *Encoder) writePrivateHeader(objectBufferLength uint32) error {
	b := make([]byte, 4)
	enc.ch.Endianness.PutUint32(b, objectBufferLength)
	if err := enc.writeBytes(b); err != nil {
		return Errorf("could not write private header object buffer length: %v", err)
	}
	// Filler bytes: MUST be set to 0 during marshaling.
	if err := enc.writeBytes([]byte{0x00, 0x00, 0x00, 0x00}); err != nil {
		return Errorf("could not write private header filler: %v", err)
	}
	return nil
}
