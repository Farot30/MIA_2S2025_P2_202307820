package bloques

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"unsafe"
)

const NullPointer int32 = -1

// BloqueApuntador estructura para apuntadores indirectos (simples, dobles y triples)
type BloqueApuntador struct {
	B_pointers [16]int32
}

// NewBloqueApuntador crea un nuevo bloque de apuntadores vacío
func NewBloqueApuntador() *BloqueApuntador {
	ba := &BloqueApuntador{}
	for i := range ba.B_pointers {
		ba.B_pointers[i] = NullPointer
	}
	return ba
}

// SetPointer establece un apuntador en la posición especificada
func (ba *BloqueApuntador) SetPointer(index int, blockNum int32) bool {
	if index < 0 || index >= 16 {
		return false
	}
	// permitir NullPointer (-1) o cualquier blockNum >= 0; validación adicional (S_blocks_count) debe hacerse fuera
	if blockNum < -1 {
		return false
	}
	ba.B_pointers[index] = blockNum
	return true
}

// GetPointer obtiene un apuntador de la posición especificada
func (ba *BloqueApuntador) GetPointer(index int) int32 {
	if index < 0 || index >= 16 {
		return -1
	}
	return ba.B_pointers[index]
}

// Clear limpia todos los apuntadores (usa el sentinel NullPointer)
func (ba *BloqueApuntador) Clear() {
	for i := range ba.B_pointers {
		ba.B_pointers[i] = NullPointer
	}
}

// IsEmpty verifica si todos los apuntadores están en NullPointer
func (ba *BloqueApuntador) IsEmpty() bool {
	for _, p := range ba.B_pointers {
		if p != NullPointer {
			return false
		}
	}
	return true
}

// ToBytes serializa el bloque a una slice de 64 bytes (LittleEndian)
func (ba *BloqueApuntador) ToBytes() []byte {
	buf := bytes.NewBuffer(make([]byte, 0, ba.GetSize()))
	for i := 0; i < len(ba.B_pointers); i++ { // CORREGIDO: ba.Bpointers -> ba.B_pointers
		binary.Write(buf, binary.LittleEndian, ba.B_pointers[i])
	}
	// asegurar longitud exacta
	b := buf.Bytes()
	if int32(len(b)) < ba.GetSize() {
		padding := make([]byte, int(ba.GetSize())-len(b))
		b = append(b, padding...)
	}
	return b
}

// FromBytes deserializa 64 bytes en el bloque (espera LittleEndian)
func (b *BloqueApuntador) FromBytes(data []byte) error {
	size := b.GetSize()
	if len(data) < int(size) {
		return fmt.Errorf("FromBytes: data too short, want %d got %d", size, len(data))
	}
	for i := 0; i < len(b.B_pointers); i++ {
		off := i * 4
		u := binary.LittleEndian.Uint32(data[off : off+4])
		if u == 0xFFFFFFFF {
			b.B_pointers[i] = -1
		} else {
			b.B_pointers[i] = int32(u)
		}
	}
	return nil
}

// GetSize retorna el tamaño del bloque en bytes (16 * 4 = 64 bytes)
func (ba *BloqueApuntador) GetSize() int32 {
	return int32(unsafe.Sizeof(*ba))
}
