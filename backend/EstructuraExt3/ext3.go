package EstructuraExt3P2

import (
	BloquesP2 "MIA_2S2025_P2_202307820/Bloques"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"strings"
	"time"
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
)

// CalcularEXT3Estructuras calcula el número de inodos y bloques para EXT3
// Fórmula:
// tamaño_particion = sizeOf(superblock) + n * sizeOf(Journaling) + n + 3 * n + n * sizeOf(inodos) + 3 * n * sizeOf(block)
// n = floor((partitionSize - SUPERBLOCK_SIZE) / (JOURNAL_SIZE + 1 + 3 + INODE_SIZE + 3*BLOCK_SIZE))
func CalcularEXT3Estructuras(partitionSize int32) (inodes int32, blocks int32) {
	// Usar GetJournalSize() en lugar de JOURNAL_SIZE
	journalSize := GetJournalSize()
	denominator := float64(journalSize + 1 + 3 + INODE_SIZE + 3*BLOCK_SIZE)
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
	journalingSize := inodes * GetJournalSize()
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
		S_filesystem_type:   EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE, // EXT3
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
	sizes["Journaling"] = inodesCount * GetJournalSize()
	sizes["BitmapInode"] = (inodesCount + 7) / 8
	sizes["BitmapBlock"] = (blocksCount + 7) / 8
	sizes["InodeTable"] = inodesCount * INODE_SIZE
	sizes["BlockTable"] = blocksCount * BLOCK_SIZE
	sizes["total_used"] = fs.CalcularTotalEspacioUsado()
	sizes["available_space"] = fs.PartitionSize - sizes["total_used"]

	return sizes
}

func (fs *EXT3FileSystem) CalcularTotalEspacioUsado() int32 {

	sizes := fs.GetStructureSizesEXT3()
	return sizes["SuperBlock"] + sizes["Journaling"] + sizes["BitmapInode"] + sizes["BitmapBlock"] + sizes["InodeTable"] + sizes["BlockTable"]
}

func (fs *EXT3FileSystem) GetPartitionLayoutEXT3() map[string]map[string]int32 {
	sizes := fs.GetStructureSizesEXT3()
	layout := make(map[string]map[string]int32)

	// SuperBloque
	layout["SuperBlock"] = map[string]int32{
		"start": fs.StartOffset,
		"size":  sizes["SuperBlock"],
		"end":   fs.StartOffset + sizes["SuperBlock"] - 1,
	}

	// Journaling
	journalingStart := fs.StartOffset + sizes["SuperBlock"]
	layout["Journaling"] = map[string]int32{
		"start": journalingStart,
		"size":  sizes["Journaling"],
		"end":   journalingStart + sizes["Journaling"] - 1,
	}

	// Bitmap de Inodos
	bitmapInodeStart := journalingStart + sizes["Journaling"]
	layout["BitmapInode"] = map[string]int32{
		"start": bitmapInodeStart,
		"size":  sizes["BitmapInode"],
		"end":   bitmapInodeStart + sizes["BitmapInode"] - 1,
	}

	// Bitmap de Bloques
	bitmapBlockStart := bitmapInodeStart + sizes["BitmapInode"]
	layout["BitmapBlock"] = map[string]int32{
		"start": bitmapBlockStart,
		"size":  sizes["BitmapBlock"],
		"end":   bitmapBlockStart + sizes["BitmapBlock"] - 1,
	}

	// Tabla de Inodos
	inodeTableStart := bitmapBlockStart + sizes["BitmapBlock"]
	layout["InodeTable"] = map[string]int32{
		"start": inodeTableStart,
		"size":  sizes["InodeTable"],
		"end":   inodeTableStart + sizes["InodeTable"] - 1,
	}

	// Tabla de Bloques
	blockTableStart := inodeTableStart + sizes["InodeTable"]
	layout["BlockTable"] = map[string]int32{
		"start": blockTableStart,
		"size":  sizes["BlockTable"],
		"end":   blockTableStart + sizes["BlockTable"] - 1,
	}

	return layout
}

func ValidarTamañoParticionEXT3(partitionSize int32) (bool, string, map[string]int32) {
	inodes, blocks := CalcularEXT3Estructuras(partitionSize)
	if inodes < 0 || blocks < 0 {
		return false, "El tamaño de la partición es insuficiente para crear un sistema de archivos EXT3.", nil
	}

	minSpace := SUPERBLOCK_SIZE +
		(inodes * GetJournalSize()) +
		((inodes + 7) / 8) +
		((blocks + 7) / 8) +
		(inodes * INODE_SIZE) +
		(blocks * BLOCK_SIZE)

	info := map[string]int32{
		"partition_size":    partitionSize,
		"min_required_size": minSpace,
		"available_space":   partitionSize - minSpace,
		"inodes_count":      inodes,
		"blocks_count":      blocks,
		"superblock_size":   SUPERBLOCK_SIZE,
		"journaling_size":   inodes * GetJournalSize(),
		"bitmap_inode_size": (inodes + 7) / 8,
		"bitmap_block_size": (blocks + 7) / 8,
		"inode_table_size":  inodes * INODE_SIZE,
		"block_table_size":  blocks * BLOCK_SIZE,
	}

	if partitionSize < minSpace {
		return false, "El tamaño de la partición es insuficiente para crear un sistema de archivos EXT3.", info
	}
	return true, "El tamaño de la partición es suficiente para crear un sistema de archivos EXT3.", info
}

func (fs *EXT3FileSystem) FormatoParticionEXT3() error {
	file, err := os.OpenFile(fs.PartitionPath, os.O_RDWR, 0666)

	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Seek(int64(fs.StartOffset), 0)
	if err != nil {
		return err
	}

	err = utilitiesP2.WriteStruct(file, int64(fs.StartOffset), fs.SuperBlock)
	if err != nil {
		return err
	}

	_, err = file.Seek(int64(fs.SuperBlock.S_bm_inode_start), 0)
	if err != nil {
		return err
	}

	bitmapInodeSize := (fs.SuperBlock.S_inodes_count + 7) / 8
	inodeBitmap := make([]byte, bitmapInodeSize)
	// Marcar el primer inodo (raíz) como usado
	if bitmapInodeSize > 0 {
		inodeBitmap[0] = 0x80 // 10000000 - primer bit en 1
	}
	_, err = file.Write(inodeBitmap)
	if err != nil {
		return err
	}

	_, err = file.Seek(int64(fs.SuperBlock.S_bm_block_start), 0)
	if err != nil {
		return err
	}

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

	// 5. Inicializar el journaling (específico de EXT3)
	journalingStart := fs.StartOffset + SUPERBLOCK_SIZE
	journalingSize := fs.SuperBlock.S_inodes_count * GetJournalSize()

	// Limpiar el área de journaling primero
	journalingData := make([]byte, journalingSize)
	_, err = file.Seek(int64(journalingStart), 0)
	if err != nil {
		return err
	}
	_, err = file.Write(journalingData)
	if err != nil {
		return err
	}

	// Agregar una entrada inicial de formateo
	initialJournal := NewJournal(1)
	copy(initialJournal.J_Content.I_Operation[:], OPERATION_FORMAT)
	copy(initialJournal.J_Content.I_Path[:], "/")
	copy(initialJournal.J_Content.I_Content[:], "Formateo inicial EXT3")
	initialJournal.J_Content.I_Date = float64(time.Now().Unix())

	err = utilitiesP2.WriteStruct(file, int64(journalingStart), initialJournal)
	if err != nil {
		return err
	}

	// 6. Crear el inodo raíz
	rootInode := fs.InicializarInodeRaiz()
	err = utilitiesP2.WriteStruct(file, int64(fs.SuperBlock.S_inode_start), rootInode)
	if err != nil {
		return err
	}

	// 7. Crear el bloque raíz del directorio
	rootBlock := fs.InicializarBloqueDirectorioRaiz()
	err = utilitiesP2.WriteStruct(file, int64(fs.SuperBlock.S_block_start), rootBlock)
	if err != nil {
		return err
	}

	return nil
}

func (fs *EXT3FileSystem) InicializarInodeRaiz() *EstructuraarchivocarpetaP2.Inodo {
	rootInode := &EstructuraarchivocarpetaP2.Inodo{}
	rootInode.SetRootValues()
	return rootInode
}

func (fs *EXT3FileSystem) InicializarBloqueDirectorioRaiz() *BloquesP2.BloqueCarpeta {
	rootBlock := &BloquesP2.BloqueCarpeta{}

	copy(rootBlock.B_content[0].B_name[:], ".")
	rootBlock.B_content[0].B_inodo = 0 // Inodo del directorio raíz

	copy(rootBlock.B_content[1].B_name[:], "..")
	rootBlock.B_content[1].B_inodo = 0 // Inodo del directorio padre (raíz)

	for i := 2; i < 4; i++ {
		rootBlock.B_content[i].B_inodo = -1 // Indicar entradas vacías
	}

	return rootBlock
}

func ReadEXT3FileSystem(partitionPath string, startOffset int32) (*EXT3FileSystem, error) {
	file, err := os.OpenFile(partitionPath, os.O_RDONLY, 0666)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	superBlock := &EstructuraarchivocarpetaP2.SuperBloque{}
	err = utilitiesP2.ReadStruct(file, int64(startOffset), superBlock)
	if err != nil {
		return nil, err
	}

	if superBlock.S_magic != EstructuraarchivocarpetaP2.EXT2_MAGIC {
		return nil, err
	}

	// Validar que sea un sistema de archivos EXT3
	if superBlock.S_magic != 0xEF53 || superBlock.S_filesystem_type != 3 {
		return nil, err
	}

	partitionSize := CalcularPartitionSizeEXT3(superBlock.S_inodes_count, superBlock.S_blocks_count)

	return &EXT3FileSystem{
		SuperBlock:    superBlock,
		PartitionPath: partitionPath,
		PartitionSize: partitionSize,
		StartOffset:   startOffset,
	}, nil
}

func CalcularPartitionSizeEXT3(inodesCount, blocksCount int32) int32 {
	superBloque := int32(SUPERBLOCK_SIZE)
	bitmapInodeSize := (inodesCount + 7) / 8
	bitmapBlockSize := (blocksCount + 7) / 8
	inodeTableSize := inodesCount * INODE_SIZE
	blockTableSize := blocksCount * BLOCK_SIZE
	journalingSize := inodesCount * GetJournalSize()

	totalSize := superBloque + journalingSize + bitmapInodeSize + bitmapBlockSize + inodeTableSize + blockTableSize
	return totalSize
}

func FormatPartitionForFDISKEXT3(partitionPath string, partititionSize int32, startOffset int32) error {

	valid, message, _ := ValidarTamañoParticionEXT3(partititionSize)
	if !valid {
		return fmt.Errorf("error de validación: %s", message)
	}

	ext3fs := CrearEXT3FileSystem(partitionPath, partititionSize, startOffset)

	return ext3fs.FormatoParticionEXT3()
}

func (fs *EXT3FileSystem) UpdateFreeSpaceEXT3(file *os.File) error {
	// Actualizar el SuperBloque en el archivo
	return utilitiesP2.WriteStruct(file, int64(fs.StartOffset), fs.SuperBlock)
}

func ExisteFormatoEXT3(partitionPath string, startOffset int32) bool {
	file, err := ReadEXT3FileSystem(partitionPath, startOffset)
	return err == nil && file != nil
}

func (fs *EXT3FileSystem) ObtenerInformacionEXT3() map[string]interface{} {
	size := fs.GetStructureSizesEXT3()
	layout := fs.GetPartitionLayoutEXT3()

	return map[string]interface{}{
		"filsesystem_type": "EXT3",
		"partition_path":   fs.PartitionPath,
		"partition_size":   fs.PartitionSize,
		"start_offset":     fs.StartOffset,
		"sperblock": map[string]interface{}{
			"inodes_count": fs.SuperBlock.S_inodes_count,
			"blocks_count": fs.SuperBlock.S_blocks_count,
			"free_blocks":  fs.SuperBlock.S_free_blocks_count,
			"free_inodes":  fs.SuperBlock.S_free_inodes_count,
			"magic":        fs.SuperBlock.S_magic,
			"mount_count":  fs.SuperBlock.S_mnt_count,
			"last_mount":   fs.SuperBlock.S_mtime,
			"last_umount":  fs.SuperBlock.S_umtime,
		},
		"sizes":  size,
		"layout": layout,
	}
}

func (fs *EXT3FileSystem) LeerInodo(index int32) (*EstructuraarchivocarpetaP2.Inodo, error) {
	file, err := os.OpenFile(fs.PartitionPath, os.O_RDONLY, 0666)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	inode := &EstructuraarchivocarpetaP2.Inodo{}
	inodeOffset := fs.SuperBlock.S_inode_start + (index * INODE_SIZE)
	err = utilitiesP2.ReadStruct(file, int64(inodeOffset), inode)
	if err != nil {
		return nil, err
	}

	return inode, nil
}

func (fs *EXT3FileSystem) LeerArchivo(filePath string) (string, error) {

	if strings.TrimSpace(filePath) == "" {
		return "", fmt.Errorf("ruta vacia")
	}

	p := strings.TrimSpace(filePath)
	if p == "/" || p == "" {
		return "", fmt.Errorf("ruta vacia")
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")

	file, err := os.OpenFile(fs.PartitionPath, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	defer file.Close()

	readBlock := func(blockNum int32) ([]byte, error) {
		if blockNum < 0 || blockNum >= fs.SuperBlock.S_blocks_count {
			return nil, fmt.Errorf("número de bloque fuera de rango")
		}
		blockOdset := int64(fs.SuperBlock.S_block_start) + int64(blockNum)*int64(fs.SuperBlock.S_block_s)
		blockData := make([]byte, fs.SuperBlock.S_block_s)
		_, err := file.ReadAt(blockData, blockOdset)
		if err != nil {
			return nil, err
		}
		return blockData, nil
	}

	var curInodeIdx int32 = 0 // Inodo raíz

	for _, comp := range parts {
		in, err := fs.LeerInodo(curInodeIdx)
		if err != nil {
			return "", fmt.Errorf("leer inodo %d: %v", curInodeIdx, err)
		}
		if !in.IsDirectory() && comp != parts[len(parts)-1] {
			return "", fmt.Errorf("componente de ruta no es un directorio: %s", comp)
		}

		found := int32(-1)

		for i := 0; i < 12 && found == -1; i++ {
			b := in.Iblock[i]
			if b < 0 || b >= fs.SuperBlock.S_blocks_count {
				continue
			}
			blockData, err := readBlock(b)
			if err != nil {
				continue
			}
			var dirBlock BloquesP2.BloqueCarpeta
			if int32(len(blockData)) >= dirBlock.GetSize() {
				copy((*[64]byte)(unsafe.Pointer(&dirBlock))[:], blockData[:dirBlock.GetSize()])
			} else {
				copy((*[64]byte)(unsafe.Pointer(&dirBlock))[:], blockData)
			}
			for _, entry := range dirBlock.B_content {
				name := strings.TrimRight(string(entry.B_name[:]), "\x00")
				if name == comp {
					found = entry.B_inodo
					break
				}
			}
		}

		if found == -1 && in.Iblock[12] >= 0 {
			ptrBlock := in.Iblock[12]
			blockData, err := readBlock(ptrBlock)
			if err == nil {
				var bap BloquesP2.BloqueApuntador
				if int32(len(blockData)) >= bap.GetSize() {
					copy((*[64]byte)(unsafe.Pointer(&bap))[:], blockData[:bap.GetSize()])
				} else {
					copy((*[64]byte)(unsafe.Pointer(&bap))[:], blockData)
				}
				for _, bnum := range bap.B_pointers {
					if bnum < 0 || bnum >= fs.SuperBlock.S_blocks_count {
						continue
					}
					blockData, err := readBlock(bnum)
					if err != nil {
						continue
					}
					var dirBlock BloquesP2.BloqueCarpeta
					if int32(len(blockData)) >= dirBlock.GetSize() {
						copy((*[64]byte)(unsafe.Pointer(&dirBlock))[:], blockData[:dirBlock.GetSize()])
					} else {
						copy((*[64]byte)(unsafe.Pointer(&dirBlock))[:], blockData)
					}
					for _, entry := range dirBlock.B_content {
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
			return "", fmt.Errorf("componente de ruta no encontrado: %s", comp)
		}
		curInodeIdx = found
	}

	targetInode, err := fs.LeerInodo(curInodeIdx)
	if err != nil {
		return "", fmt.Errorf("leer inodo final %d: %v", curInodeIdx, err)
	}
	if targetInode.IsDirectory() {
		return "", fmt.Errorf("la ruta apunta a un directorio, no a un archivo")
	}

	var fileContent strings.Builder
	for i := 0; i < 12; i++ {
		b := targetInode.Iblock[i]
		if b < 0 || b >= fs.SuperBlock.S_blocks_count {
			continue
		}
		blockData, err := readBlock(b)
		if err != nil {
			continue
		}
		fileContent.WriteString(strings.TrimRight(string(blockData), "\x00"))
	}

	if targetInode.Iblock[12] >= 0 {
		ptrBlock := targetInode.Iblock[12]
		blockData, err := readBlock(ptrBlock)
		if err == nil {
			var bap BloquesP2.BloqueApuntador
			if int32(len(blockData)) >= bap.GetSize() {
				copy((*[64]byte)(unsafe.Pointer(&bap))[:], blockData[:bap.GetSize()])
			} else {
				copy((*[64]byte)(unsafe.Pointer(&bap))[:], blockData)
			}
			for _, bnum := range bap.B_pointers {
				if bnum < 0 || bnum >= fs.SuperBlock.S_blocks_count {
					continue
				}
				blockData, err := readBlock(bnum)
				if err != nil {
					continue
				}
				fileContent.WriteString(strings.TrimRight(string(blockData), "\x00"))
			}
		}
	}

	return fileContent.String(), nil

}

func FindFreeInodeEXT3(file *os.File, ext3fs *EXT3FileSystem) (int32, error) {

	inodeBitmap := make([]byte, ext3fs.SuperBlock.S_inodes_count/8)
	_, err := file.ReadAt(inodeBitmap, int64(ext3fs.SuperBlock.S_bm_inode_start))
	if err != nil {
		return -1, fmt.Errorf("error al leer el bitmap de inodos: %v", err)
	}

	for i := int32(1); i < ext3fs.SuperBlock.S_inodes_count; i++ {
		byteIndex := i / 8
		bitIndex := i % 8
		if (inodeBitmap[byteIndex] & (1 << bitIndex)) == 0 {
			inodeBitmap[byteIndex] |= (1 << bitIndex)
			_, err := file.WriteAt(inodeBitmap, int64(ext3fs.SuperBlock.S_bm_inode_start))
			if err != nil {
				return -1, fmt.Errorf("error al actualizar el bitmap de inodos: %v", err)
			}
			return i, nil
		}
	}

	return -1, fmt.Errorf("no hay inodos libres disponibles")
}
