package EstructuraExt3P2

import (
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	"unsafe"
)

type EXT3FileSystem struct {
	SuperBlock    *EstructuraarchivocarpetaP2.SuperBloque
	PartitionPath string
	PartitionSize int32
	StartOffset   int32
}

const (
	SUPERBLOCK_SIZE = int32(unsafe.Sizeof(EstructuraarchivocarpetaP2.SuperBloque{}))
	INODE_SIZE      = int32(unsafe.Sizeof(EstructuraarchivocarpetaP2.Inodo{}))
	BLOCK_SIZE      = 64
	BITMAP_BYTE     = 1
	JOURNAL_SIZE    = 50 // Tamaño fijo del journaling para cada inodo
)

// CalcularEXT3Estructuras calcula el número de inodos y bloques para EXT3
// Fórmula:
// tamaño_particion = sizeOf(superblock) + n * sizeOf(Journaling) + n + 3 * n + n * sizeOf(inodos) + 3 * n * sizeOf(block)
// n = floor((partitionSize - SUPERBLOCK_SIZE) / (JOURNAL_SIZE + 1 + 3 + INODE_SIZE + 3*BLOCK_SIZE))
func CalcularEXT3Estructuras(partitionSize int32) (inodes int32, blocks int32) {
	denominator := float64(JOURNAL_SIZE + 1 + 3 + INODE_SIZE + 3*BLOCK_SIZE)
	numerator := float64(partitionSize - SUPERBLOCK_SIZE)
	n := int32(numerator / denominator)
	if n < 0 {
		n = 0
	}
	inodes = n
	blocks = 3 * n
	return inodes, blocks
}

func CrearEXT3FileSystem(partitionPath string, partitionSize int32, startOffset int32) *EXT3FileSystem {
	inodes, blocks := CalcularEXT3Estructuras(partitionSize)
	currentTime := EstructuraarchivocarpetaP2.GetCurrentTimestamp()

	// Calcular offsets
	currentOffset := SUPERBLOCK_SIZE

	// Journaling (n * JOURNAL_SIZE)
	journalingSize := inodes * JOURNAL_SIZE
	currentOffset += journalingSize

	// Bitmap de inodos
	bitmapInodeStart := startOffset + currentOffset
	bitmapInodeSize := (inodes + 7) / 8 // Redondear hacia arriba
	currentOffset += bitmapInodeSize

	// Bitmap de bloques
	bitmapBlockStart := startOffset + currentOffset
	bitmapBlockSize := (blocks + 7) / 8 // Redondear hacia arriba
	currentOffset += bitmapBlockSize

	// Tabla de inodos
	inodeTableStart := startOffset + currentOffset
	inodeTableSize := inodes * INODE_SIZE
	currentOffset += inodeTableSize

	// Tabla de bloques
	blockTableStart := startOffset + currentOffset

	superBlock := &EstructuraarchivocarpetaP2.SuperBloque{
		S_filesystem_type:   3, // EXT3
		S_inodes_count:      inodes,
		S_blocks_count:      blocks,
		S_free_blocks_count: blocks - 1,
		S_free_inodes_count: inodes - 1,
		S_mtime:             currentTime,
		S_umtime:            currentTime,
		S_mnt_count:         0,
		S_magic:             0xEF53,
		S_inode_s:           INODE_SIZE,
		S_block_s:           BLOCK_SIZE,
		S_firts_ino:         2,
		S_first_blo:         1,
		S_bm_inode_start:    bitmapInodeStart,
		S_bm_block_start:    bitmapBlockStart,
		S_inode_start:       inodeTableStart,
		S_block_start:       blockTableStart,
	}

	return &EXT3FileSystem{
		SuperBlock:    superBlock,
		PartitionPath: partitionPath,
		PartitionSize: partitionSize,
		StartOffset:   startOffset,
	}
}

func (fs *EXT3FileSystem) GetStructureSizesEXT3() map[string]int32 {
	inodesCount := fs.SuperBlock.S_inodes_count
	blocksCount := fs.SuperBlock.S_blocks_count

	sizes := make(map[string]int32)
	sizes["SuperBlock"] = SUPERBLOCK_SIZE
	sizes["Journaling"] = inodesCount * JOURNAL_SIZE
	sizes["BitmapInode"] = (inodesCount + 7) / 8
	sizes["BitmapBlock"] = (blocksCount + 7) / 8
	sizes["InodeTable"] = inodesCount * INODE_SIZE
	sizes["BlockTable"] = blocksCount * BLOCK_SIZE

	return sizes
}
