package EstructuraExt2P2

import (
	BloquesP2 "MIA_2S2025_P2_202307820/Bloques"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"unsafe"
)

// EXT2FileSystem representa el sistema de archivos EXT2
type EXT2FileSystem struct {
	SuperBlock    *EstructuraarchivocarpetaP2.SuperBloque
	PartitionPath string
	PartitionSize int32
	StartOffset   int32 // Offset donde inicia la partición en el disco
}

// EXT2Constants contiene las constantes para el cálculo del sistema EXT2
const (
	SUPERBLOCK_SIZE = int32(unsafe.Sizeof(EstructuraarchivocarpetaP2.SuperBloque{}))
	INODE_SIZE      = int32(unsafe.Sizeof(EstructuraarchivocarpetaP2.Inodo{}))
	BLOCK_SIZE      = 64 // Tamaño de cada bloque (carpeta, archivo, apuntador)
	BITMAP_BYTE     = 1  // 1 byte para 8 bits del bitmap
)

// CalculateEXT2Structures calcula el número de estructuras para EXT2
// Fórmula: tamaño_particion = sizeof(superblock) + n + 3*n + n*sizeof(inodos) + 3*n*sizeof(block)
// Donde n es el número de inodos y 3*n es el número de bloques
func CalculateEXT2Structures(partitionSize int32) (inodes int32, blocks int32) {
	// Reorganizar la ecuación para despejar n:
	// partitionSize = SUPERBLOCK_SIZE + n + 3*n + n*INODE_SIZE + 3*n*BLOCK_SIZE
	// partitionSize = SUPERBLOCK_SIZE + n*(1 + 3 + INODE_SIZE + 3*BLOCK_SIZE)
	// n = (partitionSize - SUPERBLOCK_SIZE) / (1 + 3 + INODE_SIZE + 3*BLOCK_SIZE)

	denominator := float64(1 + 3 + INODE_SIZE + 3*BLOCK_SIZE)
	numerator := float64(partitionSize - SUPERBLOCK_SIZE)

	n := math.Floor(numerator / denominator)

	inodes = int32(n)
	blocks = 3 * inodes // El número de bloques es el triple del número de inodos

	return inodes, blocks
}

// CreateEXT2FileSystem crea un nuevo sistema de archivos EXT2
func CreateEXT2FileSystem(partitionPath string, partitionSize int32, startOffset int32) *EXT2FileSystem {
	// Calcular el número de inodos y bloques
	inodesCount, blocksCount := CalculateEXT2Structures(partitionSize)

	currentTime := EstructuraarchivocarpetaP2.GetCurrentTimestamp() // Obtener el timestamp actual

	// Crear el superbloque
	superBlock := &EstructuraarchivocarpetaP2.SuperBloque{
		S_filesystem_type:   EstructuraarchivocarpetaP2.EXT2_FILESYSTEM_TYPE,
		S_inodes_count:      inodesCount,
		S_blocks_count:      blocksCount,
		S_free_blocks_count: blocksCount - 1, // -1 por el bloque raíz
		S_free_inodes_count: inodesCount - 1, // -1 por el inodo raíz
		S_mtime:             currentTime,
		S_umtime:            currentTime,
		S_mnt_count:         0,
		S_magic:             EstructuraarchivocarpetaP2.EXT2_MAGIC,
		S_inode_s:           INODE_SIZE,
		S_block_s:           BLOCK_SIZE,
		S_firts_ino:         2, // El primer inodo libre (1 es el raíz)
		S_first_blo:         1, // El primer bloque libre (0 es reservado)
	}

	// Calcular las posiciones de las estructuras dentro de la partición
	currentOffset := SUPERBLOCK_SIZE

	// Bitmap de inodos (n bits = n/8 bytes)
	superBlock.S_bm_inode_start = startOffset + currentOffset
	bitmapInodeSize := (inodesCount + 7) / 8 // Redondear hacia arriba
	currentOffset += bitmapInodeSize

	// Bitmap de bloques (3*n bits = 3*n/8 bytes)
	superBlock.S_bm_block_start = startOffset + currentOffset
	bitmapBlockSize := (blocksCount + 7) / 8 // Redondear hacia arriba
	currentOffset += bitmapBlockSize

	// Tabla de inodos (n * sizeof(inodo))
	superBlock.S_inode_start = startOffset + currentOffset
	inodeTableSize := inodesCount * INODE_SIZE
	currentOffset += inodeTableSize

	// Tabla de bloques (3*n * sizeof(bloque))
	superBlock.S_block_start = startOffset + currentOffset

	return &EXT2FileSystem{
		SuperBlock:    superBlock,
		PartitionPath: partitionPath,
		PartitionSize: partitionSize,
		StartOffset:   startOffset,
	}
}

// GetStructureSizes retorna los tamaños de cada estructura del sistema EXT2
func (fs *EXT2FileSystem) GetStructureSizes() map[string]int32 {
	inodesCount := fs.SuperBlock.S_inodes_count
	blocksCount := fs.SuperBlock.S_blocks_count

	return map[string]int32{
		"superblock":      SUPERBLOCK_SIZE,
		"bitmap_inodes":   (inodesCount + 7) / 8,
		"bitmap_blocks":   (blocksCount + 7) / 8,
		"inode_table":     inodesCount * INODE_SIZE,
		"block_table":     blocksCount * BLOCK_SIZE,
		"total_used":      fs.CalculateTotalUsedSpace(),
		"available_space": fs.PartitionSize - fs.CalculateTotalUsedSpace(),
	}
}

// CalculateTotalUsedSpace calcula el espacio total usado por las estructuras
func (fs *EXT2FileSystem) CalculateTotalUsedSpace() int32 {
	sizes := fs.GetStructureSizes()
	return sizes["superblock"] + sizes["bitmap_inodes"] + sizes["bitmap_blocks"] +
		sizes["inode_table"] + sizes["block_table"]
}

// GetPartitionLayout retorna la distribución de la partición
func (fs *EXT2FileSystem) GetPartitionLayout() map[string]map[string]int32 {
	sizes := fs.GetStructureSizes()

	layout := map[string]map[string]int32{
		"superblock": {
			"start": fs.StartOffset,
			"size":  sizes["superblock"],
			"end":   fs.StartOffset + sizes["superblock"] - 1,
		},
		"bitmap_inodes": {
			"start": fs.SuperBlock.S_bm_inode_start,
			"size":  sizes["bitmap_inodes"],
			"end":   fs.SuperBlock.S_bm_inode_start + sizes["bitmap_inodes"] - 1,
		},
		"bitmap_blocks": {
			"start": fs.SuperBlock.S_bm_block_start,
			"size":  sizes["bitmap_blocks"],
			"end":   fs.SuperBlock.S_bm_block_start + sizes["bitmap_blocks"] - 1,
		},
		"inode_table": {
			"start": fs.SuperBlock.S_inode_start,
			"size":  sizes["inode_table"],
			"end":   fs.SuperBlock.S_inode_start + sizes["inode_table"] - 1,
		},
		"block_table": {
			"start": fs.SuperBlock.S_block_start,
			"size":  sizes["block_table"],
			"end":   fs.SuperBlock.S_block_start + sizes["block_table"] - 1,
		},
	}

	return layout
}

// ValidatePartitionSize verifica si el tamaño de la partición es suficiente
func ValidatePartitionSize(partitionSize int32) (bool, string, map[string]int32) {
	inodesCount, blocksCount := CalculateEXT2Structures(partitionSize)

	if inodesCount <= 0 || blocksCount <= 0 {
		return false, "La partición es demasiado pequeña para el sistema EXT2", nil
	}

	// Calcular espacio mínimo requerido
	minSpace := SUPERBLOCK_SIZE +
		(inodesCount+7)/8 + // Bitmap inodos
		(blocksCount+7)/8 + // Bitmap bloques
		inodesCount*INODE_SIZE + // Tabla inodos
		blocksCount*BLOCK_SIZE // Tabla bloques

	info := map[string]int32{
		"partition_size":     partitionSize,
		"min_required":       minSpace,
		"available_space":    partitionSize - minSpace,
		"inodes_count":       inodesCount,
		"blocks_count":       blocksCount,
		"superblock_size":    SUPERBLOCK_SIZE,
		"bitmap_inodes_size": (inodesCount + 7) / 8,
		"bitmap_blocks_size": (blocksCount + 7) / 8,
		"inode_table_size":   inodesCount * INODE_SIZE,
		"block_table_size":   blocksCount * BLOCK_SIZE,
	}

	if partitionSize < minSpace {
		return false, "La partición es demasiado pequeña para las estructuras calculadas", info
	}

	return true, "La partición tiene suficiente espacio para EXT2", info
}

// FormatPartition formatea una partición con el sistema EXT2
func (fs *EXT2FileSystem) FormatPartition() error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(fs.PartitionPath, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	// 1. Escribir el superbloque al inicio de la partición
	_, err = file.Seek(int64(fs.StartOffset), 0)
	if err != nil {
		return err
	}

	err = utilitiesP2.WriteStruct(file, int64(fs.StartOffset), fs.SuperBlock)
	if err != nil {
		return err
	}

	// 2. Inicializar el bitmap de inodos (todos en 0 = libre, excepto el inodo raíz)
	_, err = file.Seek(int64(fs.SuperBlock.S_bm_inode_start), 0)
	if err != nil {
		return err
	}

	// Crear bitmap de inodos - primer bit en 1 para inodo raíz
	bitmapInodeSize := (fs.SuperBlock.S_inodes_count + 7) / 8
	inodeBitmap := make([]byte, bitmapInodeSize)
	// Marcar el primer inodo (raíz) como usado
	if bitmapInodeSize > 0 {
		inodeBitmap[0] = 0x80 // 10000000 return nil- primer bit en 1
	}
	_, err = file.Write(inodeBitmap)
	if err != nil {
		return err
	}

	// 3. Inicializar el bitmap de bloques (todos en 0 = libre, excepto el bloque raíz)
	_, err = file.Seek(int64(fs.SuperBlock.S_bm_block_start), 0)
	if err != nil {
		return err
	}

	// Crear bitmap de bloques - primer bit en 1 para bloque raíz
	bitmapBlockSize := (fs.SuperBlock.S_blocks_count + 7) / 8
	blockBitmap := make([]byte, bitmapBlockSize)
	// Marcar el primer bloque (raíz) como usado
	if bitmapBlockSize > 0 {
		blockBitmap[0] = 0x80 // 10000000 - primer bit en 1
	}
	_, err = file.Write(blockBitmap)
	if err != nil {
		return err
	}

	// 4. Crear el inodo raíz usando la función auxiliar
	rootInode := fs.InitializeRootInode()
	err = utilitiesP2.WriteStruct(file, int64(fs.SuperBlock.S_inode_start), rootInode)
	if err != nil {
		return err
	}

	// 5. Crear el bloque raíz usando la función auxiliar
	rootBlock := fs.InitializeRootDirectoryBlock()
	err = utilitiesP2.WriteStruct(file, int64(fs.SuperBlock.S_block_start), rootBlock)
	if err != nil {
		return err
	}

	return nil
}

// / InitializeRootInode inicializa el inodo raíz con valores por defecto
func (fs *EXT2FileSystem) InitializeRootInode() *EstructuraarchivocarpetaP2.Inodo {
	rootInode := &EstructuraarchivocarpetaP2.Inodo{}
	rootInode.SetRootValues()
	return rootInode
}

// InitializeRootDirectoryBlock inicializa el bloque del directorio raíz
func (fs *EXT2FileSystem) InitializeRootDirectoryBlock() *BloquesP2.BloqueCarpeta {
	rootBlock := &BloquesP2.BloqueCarpeta{}

	// Entrada para "." (directorio actual)
	copy(rootBlock.B_content[0].B_name[:], ".")
	rootBlock.B_content[0].B_inodo = 0 // Apunta a sí mismo

	// Entrada para ".." (directorio padre - en raíz apunta a sí mismo)
	copy(rootBlock.B_content[1].B_name[:], "..")
	rootBlock.B_content[1].B_inodo = 0 // En raíz apunta a sí mismo

	// Las demás entradas están vacías (B_inodo = -1)
	for i := 2; i < 4; i++ {
		rootBlock.B_content[i].B_inodo = -1
	}

	return rootBlock
}

// ReadEXT2FileSystem lee un sistema de archivos EXT2 existente desde disco
func ReadEXT2FileSystem(partitionPath string, startOffset int32) (*EXT2FileSystem, error) {
	file, err := os.OpenFile(partitionPath, os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Leer el superbloque
	superBlock := &EstructuraarchivocarpetaP2.SuperBloque{}
	err = utilitiesP2.ReadStruct(file, int64(startOffset), superBlock)
	if err != nil {
		return nil, err
	}

	// Verificar la magia del sistema de archivos
	if superBlock.S_magic != EstructuraarchivocarpetaP2.EXT2_MAGIC {
		return nil, err
	}

	// Determinar el tamaño de la partición basándose en las estructuras
	partitionSize := CalculateTotalPartitionSize(superBlock.S_inodes_count, superBlock.S_blocks_count)

	return &EXT2FileSystem{
		SuperBlock:    superBlock,
		PartitionPath: partitionPath,
		PartitionSize: partitionSize,
		StartOffset:   startOffset,
	}, nil
}

// CalculateTotalPartitionSize calcula el tamaño total necesario para una partición EXT2
func CalculateTotalPartitionSize(inodesCount, blocksCount int32) int32 {
	superblockSize := int32(SUPERBLOCK_SIZE)
	bitmapInodeSize := (inodesCount + 7) / 8
	bitmapBlockSize := (blocksCount + 7) / 8
	inodeTableSize := inodesCount * INODE_SIZE
	blockTableSize := blocksCount * BLOCK_SIZE

	return superblockSize + bitmapInodeSize + bitmapBlockSize + inodeTableSize + blockTableSize
}

// FormatPartitionForFDISK formatea una partición cuando se usa con FDISK
func FormatPartitionForFDISK(partitionPath string, partitionSize int32, startOffset int32) error {
	// Validar que la partición sea suficiente
	valid, message, _ := ValidatePartitionSize(partitionSize)
	if !valid {
		return fmt.Errorf("error en validación: %s", message)
	}

	// Crear el sistema EXT2
	ext2fs := CreateEXT2FileSystem(partitionPath, partitionSize, startOffset)

	// Formatear la partición
	return ext2fs.FormatPartition()
}

// IsEXT2Formatted verifica si una partición ya tiene formato EXT2
func IsEXT2Formatted(partitionPath string, startOffset int32) bool {
	fs, err := ReadEXT2FileSystem(partitionPath, startOffset)
	return err == nil && fs != nil
}

// GetEXT2Info retorna información detallada del sistema EXT2
func (fs *EXT2FileSystem) GetEXT2Info() map[string]interface{} {
	sizes := fs.GetStructureSizes()
	layout := fs.GetPartitionLayout()

	return map[string]interface{}{
		"filesystem_type": "EXT2",
		"partition_path":  fs.PartitionPath,
		"partition_size":  fs.PartitionSize,
		"start_offset":    fs.StartOffset,
		"superblock": map[string]interface{}{
			"inodes_count":     fs.SuperBlock.S_inodes_count,
			"blocks_count":     fs.SuperBlock.S_blocks_count,
			"free_inodes":      fs.SuperBlock.S_free_inodes_count,
			"free_blocks":      fs.SuperBlock.S_free_blocks_count,
			"magic":            fs.SuperBlock.S_magic,
			"mount_count":      fs.SuperBlock.S_mnt_count,
			"first_free_inode": fs.SuperBlock.S_firts_ino,
			"first_free_block": fs.SuperBlock.S_first_blo,
		},
		"sizes":  sizes,
		"layout": layout,
	}
}

// UpdateFreeSpace actualiza los contadores de espacio libre
func (fs *EXT2FileSystem) UpdateFreeSpace(file *os.File) error {
	// Escribir el superbloque actualizado
	return utilitiesP2.WriteStruct(file, int64(fs.StartOffset), fs.SuperBlock)
}

func (fs *EXT2FileSystem) ReadInode(index int32) (*EstructuraarchivocarpetaP2.Inodo, error) {
	file, err := os.OpenFile(fs.PartitionPath, os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	inode := &EstructuraarchivocarpetaP2.Inodo{}
	offset := int64(fs.SuperBlock.S_inode_start) + int64(index)*int64(INODE_SIZE)
	err = utilitiesP2.ReadStruct(file, offset, inode)
	if err != nil {
		return nil, err
	}
	return inode, nil
}

func (fs *EXT2FileSystem) ReadFile(filePath string) (string, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", fmt.Errorf("ruta vacía")
	}
	// normalizar ruta y obtener componentes
	p := strings.TrimSpace(filePath)
	if p == "/" || p == "" {
		return "", fmt.Errorf("ruta apunta a root o vacía")
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")

	file, err := os.OpenFile(fs.PartitionPath, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// función auxiliar: leer bloque raw
	readBlock := func(blockNum int32) ([]byte, error) {
		if blockNum < 0 || blockNum >= fs.SuperBlock.S_blocks_count {
			return nil, fmt.Errorf("bloque fuera de rango: %d", blockNum)
		}
		offset := int64(fs.SuperBlock.S_block_start) + int64(blockNum)*int64(fs.SuperBlock.S_block_s)
		buf := make([]byte, fs.SuperBlock.S_block_s)
		_, err := file.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return nil, err
		}
		return buf, nil
	}

	// comenzar en el inodo raíz (0)
	var curInodeIdx int32 = 0

	// recorrer cada componente para resolver su inodo
	for _, comp := range parts {
		// leer inodo actual
		in, err := fs.ReadInode(curInodeIdx)
		if err != nil {
			return "", fmt.Errorf("leer inodo %d: %v", curInodeIdx, err)
		}
		// si no es directorio y no estamos en el último componente, error
		if !in.IsDirectory() && comp != parts[len(parts)-1] {
			return "", fmt.Errorf("componente %s: no es directorio", comp)
		}

		found := int32(-1)

		// buscar en bloques directos de carpeta
		for i := 0; i < 12 && found == -1; i++ {
			b := in.Iblock[i]
			if b < 0 || b >= fs.SuperBlock.S_blocks_count {
				continue
			}
			buf, err := readBlock(b)
			if err != nil {
				continue
			}
			var bc BloquesP2.BloqueCarpeta
			if int32(len(buf)) >= bc.GetSize() {
				copy((*[64]byte)(unsafe.Pointer(&bc))[:], buf[:bc.GetSize()])
			} else {
				copy((*[64]byte)(unsafe.Pointer(&bc))[:], buf)
			}
			for _, entry := range bc.B_content {
				name := strings.TrimRight(string(entry.B_name[:]), "\x00")
				if name == comp {
					found = entry.B_inodo
					break
				}
			}
		}

		// si no se encontró en directos, revisar indirecto simple (i_block[12])
		if found == -1 && in.Iblock[12] >= 0 {
			ptrBlock := in.Iblock[12]
			buf, err := readBlock(ptrBlock)
			if err == nil {
				var bap BloquesP2.BloqueApuntador
				if int32(len(buf)) >= bap.GetSize() {
					copy((*[64]byte)(unsafe.Pointer(&bap))[:], buf[:bap.GetSize()])
				} else {
					copy((*[64]byte)(unsafe.Pointer(&bap))[:], buf)
				}
				for _, ptr := range bap.B_pointers {
					if ptr < 0 || ptr >= fs.SuperBlock.S_blocks_count {
						continue
					}
					bbuf, err := readBlock(ptr)
					if err != nil {
						continue
					}
					var bc BloquesP2.BloqueCarpeta
					if int32(len(bbuf)) >= bc.GetSize() {
						copy((*[64]byte)(unsafe.Pointer(&bc))[:], bbuf[:bc.GetSize()])
					} else {
						copy((*[64]byte)(unsafe.Pointer(&bc))[:], bbuf)
					}
					for _, entry := range bc.B_content {
						name := strings.TrimRight(string(entry.B_name[:]), "\x00")
						if name == comp {
							found = entry.B_inodo
							break
						}
					}
					if found != -1 {
						break
					}
				}
			}
		}

		if found == -1 {
			return "", fmt.Errorf("componente no encontrado: %s", comp)
		}
		curInodeIdx = found
	}

	// ahora curInodeIdx es el inodo del archivo objetivo
	targetInode, err := fs.ReadInode(curInodeIdx)
	if err != nil {
		return "", fmt.Errorf("leer inodo final %d: %v", curInodeIdx, err)
	}
	if !targetInode.IsFile() {
		return "", fmt.Errorf("ruta no es un archivo")
	}

	// leer datos del archivo (directos)
	var out strings.Builder
	for i := 0; i < 12; i++ {
		b := targetInode.Iblock[i]
		if b < 0 || b >= fs.SuperBlock.S_blocks_count {
			continue
		}
		buf, err := readBlock(b)
		if err != nil {
			continue
		}
		out.WriteString(strings.TrimRight(string(buf), "\x00"))
	}

	// indirecto simple
	if targetInode.Iblock[12] >= 0 {
		buf, err := readBlock(targetInode.Iblock[12])
		if err == nil {
			var bap BloquesP2.BloqueApuntador
			if int32(len(buf)) >= bap.GetSize() {
				copy((*[64]byte)(unsafe.Pointer(&bap))[:], buf[:bap.GetSize()])
			} else {
				copy((*[64]byte)(unsafe.Pointer(&bap))[:], buf)
			}
			for _, ptr := range bap.B_pointers {
				if ptr < 0 || ptr >= fs.SuperBlock.S_blocks_count {
					continue
				}
				dbuf, err := readBlock(ptr)
				if err != nil {
					continue
				}
				out.WriteString(strings.TrimRight(string(dbuf), "\x00"))
			}
		}
	}

	return out.String(), nil
}

// FindFreeBlockEXT2 busca un bloque libre en el sistema de archivos EXT2
func FindFreeBlockEXT2(file *os.File, ext2fs *EXT2FileSystem) (int32, error) {
	// Si hay un bloque libre conocido, usarlo
	if ext2fs.SuperBlock.S_first_blo != -1 {
		freeBlock := ext2fs.SuperBlock.S_first_blo

		// Buscar el siguiente bloque libre para actualizar S_first_blo
		found := false
		for i := freeBlock + 1; i < ext2fs.SuperBlock.S_blocks_count && !found; i++ {
			bytePos := i / 8
			bitPos := i % 8

			bitmap, err := utilitiesP2.ReadBytes(file, int64(ext2fs.SuperBlock.S_bm_block_start+bytePos), 1)
			if err == nil && (bitmap[0]&(1<<uint(bitPos))) == 0 {
				ext2fs.SuperBlock.S_first_blo = i
				found = true
			}
		}

		// Si no hay más bloques libres
		if !found {
			ext2fs.SuperBlock.S_first_blo = -1
		}

		return freeBlock, nil
	}

	// Buscar un bloque libre en el bitmap
	for i := int32(0); i < ext2fs.SuperBlock.S_blocks_count; i++ {
		bytePos := i / 8
		bitPos := i % 8

		bitmap, err := utilitiesP2.ReadBytes(file, int64(ext2fs.SuperBlock.S_bm_block_start+bytePos), 1)
		if err == nil && (bitmap[0]&(1<<uint(bitPos))) == 0 {
			// Encontrado un bloque libre
			return i, nil
		}
	}

	return -1, fmt.Errorf("no hay bloques libres disponibles")
}
