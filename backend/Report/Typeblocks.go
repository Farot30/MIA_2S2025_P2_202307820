package report

import (
	bloques "MIA_2S2025_P1_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P1_202307820/EstructuraExt2"
	"fmt"
	"os"
)

type BlockType int

const (
	BlockUnknown BlockType = iota
	BlockCarpeta
	BlockArchivo
	BlockApuntador
)

// Recursivamente marca los bloques referenciados por apuntadores
// Ahora recibe "level": 1 = indirecto simple, 2 = doble, 3 = triple
func markBlocksRecursively(ext2fs *EstructuraExt2.EXT2FileSystem, blockTypes map[int32]BlockType, blockNum int32, level int, origTipo BlockType, depth int) {
	// helper: validar rango de bloque
	isValidBlock := func(n int32) bool {
		return n >= ext2fs.SuperBlock.S_first_blo && n < ext2fs.SuperBlock.S_blocks_count
	}

	if blockNum == -1 || depth > 5 { // Evita loops infinitos
		return
	}
	if !isValidBlock(blockNum) {
		return
	}
	// Si este bloque es parte de una cadena de apuntadores lo marcamos como apuntador
	if level >= 1 {
		blockTypes[blockNum] = BlockApuntador
	} else {
		// nivel 0: bloque de datos (no se usa en las llamadas iniciales)
		if origTipo == BlockCarpeta || origTipo == BlockArchivo {
			blockTypes[blockNum] = origTipo
		}
	}

	// Si es un bloque apuntador, recorre sus hijos
	if level >= 1 {
		file, err := os.OpenFile(ext2fs.PartitionPath, os.O_RDONLY, 0666)
		if err != nil {
			return
		}
		defer file.Close()
		offset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
		block := make([]byte, ext2fs.SuperBlock.S_block_s)
		if _, err := file.Seek(offset, 0); err != nil {
			return
		}
		if _, err := file.Read(block); err != nil {
			return
		}
		bloqueApuntador := &bloques.BloqueApuntador{}
		// Use FromBytes to parse consistentemente
		if err := bloqueApuntador.FromBytes(block[:bloqueApuntador.GetSize()]); err == nil {
			// Contar punteros válidos antes de propagar (evita interpretar texto como punteros)
			validCnt := 0
			for _, ptr := range bloqueApuntador.B_pointers {
				if isValidBlock(ptr) {
					validCnt++
				}
			}
			if validCnt < 1 {
				// No hay punteros válidos -> no propagamos
				return
			}
			// Para cada puntero: si level-1 == 0 son bloques de datos (marcar con origTipo)
			// si level-1 > 0 siguen siendo bloques apuntador (marcar y recursar)
			nextLevel := level - 1
			for _, ptr := range bloqueApuntador.B_pointers {
				if ptr == -1 {
					continue
				}
				if !isValidBlock(ptr) {
					// puntero fuera de rango: debug y saltar
					fmt.Printf("DEBUG: markBlocksRecursively: bloque %d contiene puntero fuera de rango -> %d\n", blockNum, ptr)
					continue
				}
				if nextLevel >= 1 {
					// hijo es otro bloque apuntador
					if blockTypes[ptr] == BlockUnknown {
						blockTypes[ptr] = BlockApuntador
						markBlocksRecursively(ext2fs, blockTypes, ptr, nextLevel, origTipo, depth+1)
					} else if blockTypes[ptr] == BlockApuntador {
						markBlocksRecursively(ext2fs, blockTypes, ptr, nextLevel, origTipo, depth+1)
					}
				} else {
					// hijo es bloque de datos: marcar con el tipo original (archivo/carpeta)
					if blockTypes[ptr] == BlockUnknown {
						blockTypes[ptr] = origTipo
					}
				}
			}
		} else {
			// FromBytes falló: no propagar
			return
		}
	}
}

// Mapea todos los bloques usados y su tipo, incluyendo los referenciados por apuntadores
func MapBlockTypes(ext2fs *EstructuraExt2.EXT2FileSystem) map[int32]BlockType {
	blockTypes := make(map[int32]BlockType)
	for i := int32(0); i < ext2fs.SuperBlock.S_blocks_count; i++ {
		blockTypes[i] = BlockUnknown
	}

	// helper: validar rango
	isValidBlock := func(n int32) bool {
		return n >= ext2fs.SuperBlock.S_first_blo && n < ext2fs.SuperBlock.S_blocks_count
	}

	for i := int32(0); i < ext2fs.SuperBlock.S_inodes_count; i++ {
		inode, err := ext2fs.ReadInode(i)
		if err != nil || inode.Itype > 1 {
			continue
		}
		var tipo BlockType
		if inode.Itype == 0 {
			tipo = BlockCarpeta
		} else if inode.Itype == 1 {
			tipo = BlockArchivo
		}
		// Directos
		for j := 0; j < 12; j++ {
			b := inode.Iblock[j]
			if b != -1 && b != 0 && isValidBlock(b) {
				blockTypes[b] = tipo
			}
		}
		// Indirectos (simple, doble, triple)
		for j := 12; j < 15; j++ {
			b := inode.Iblock[j]
			if b > 0 {
				if isValidBlock(b) {
					// Marcar el bloque inicial como apuntador
					blockTypes[b] = BlockApuntador
					// level: j=12 -> 1 (simple), j=13 -> 2 (doble), j=14 -> 3 (triple)
					level := j - 11
					// recorrer recursivamente: le pasamos el nivel y el tipo original
					markBlocksRecursively(ext2fs, blockTypes, b, level, tipo, 1)
				} else {
					// puntero fuera de rango real; opcional DEBUG
					// fmt.Printf("DEBUG: MapBlockTypes: inodo %d tiene apuntador fuera de rango en Iblock[%d] -> %d\n", i, j, b)
				}
			}
		}
	}
	return blockTypes
}
