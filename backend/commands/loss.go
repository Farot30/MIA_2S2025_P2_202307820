package commandsP2

import (
	EstructuraExt2P2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	sessionP2 "MIA_2S2025_P2_202307820/Session"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"strings"
	"time"
)

// LOSS implementa el comando para simular la pérdida del sistema de archivos
func LOSS(args []string) {
	// Parsear parámetros
	params := utilitiesP2.ParseParameters(args)

	// Validar parámetro obligatorio -id
	partitionID, exists := params["id"]
	if !exists || strings.TrimSpace(partitionID) == "" {
		fmt.Println("Error: Debe especificar el parámetro obligatorio -id")
		fmt.Println("Uso: loss -id=<partition_id>")
		fmt.Println("Ejemplo: loss -id=061Disco1")
		return
	}

	partitionID = strings.ToUpper(strings.Trim(partitionID, "\""))

	// Verificar que la partición esté montada
	mountedPartition, exists := GetMountedPartition(partitionID)
	if !exists {
		fmt.Printf("Error: La partición con ID '%s' no está montada\n", partitionID)
		fmt.Println("Use el comando 'mount' para montar la partición antes de simular la pérdida")
		return
	}

	// Verificar que el usuario tenga permisos (solo root puede simular pérdida)
	if sessionP2.IsSessionActive() {
		sessionInfo := sessionP2.GetCurrentSession()
		if sessionInfo.Username != "root" {
			fmt.Printf("Error: Solo el usuario root puede simular la pérdida del sistema\n")
			fmt.Printf("Usuario actual: %s\n", sessionInfo.Username)
			return
		}
	}

	// Detectar el tipo de sistema de archivos
	fsType, err := detectFileSystemType(mountedPartition)
	if err != nil {
		fmt.Printf("Error detectando el sistema de archivos: %s\n", err)
		return
	}

	fmt.Printf("=== SIMULANDO PÉRDIDA DEL SISTEMA DE ARCHIVOS ===\n")
	fmt.Printf("Partición: %s\n", partitionID)
	fmt.Printf("Disco: %s\n", mountedPartition.Path)
	fmt.Printf("Tipo: %s\n", mountedPartition.Type)
	fmt.Printf("Sistema de archivos: %s\n", getFileSystemTypeName(fsType))
	fmt.Printf("ADVERTENCIA: Esta operación causará pérdida de datos!\n")
	fmt.Printf("Iniciando simulación de pérdida...\n\n")

	// Ejecutar la simulación de pérdida según el tipo de sistema de archivos
	switch fsType {
	case EstructuraarchivocarpetaP2.EXT2_FILESYSTEM_TYPE:
		err = simulateLossEXT2(mountedPartition)
	case EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE:
		err = simulateLossEXT3(mountedPartition)
	default:
		fmt.Printf("Error: Tipo de sistema de archivos no soportado: %s\n", getFileSystemTypeName(fsType))
		return
	}

	if err != nil {
		fmt.Printf("Error durante la simulación de pérdida: %s\n", err)
		return
	}

	fmt.Printf("\n=== SIMULACIÓN DE PÉRDIDA COMPLETADA ===\n")
	fmt.Printf("Los siguientes bloques han sido limpiados con \\0:\n")
	fmt.Printf("✓ Bitmap de Inodos\n")
	fmt.Printf("✓ Bitmap de Bloques\n")
	fmt.Printf("✓ Área de Inodos\n")
	fmt.Printf("✓ Área de Bloques\n")
	fmt.Printf("Hora de finalización: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("NOTA: Use el comando 'recovery' para intentar recuperar el sistema\n")
}

// simulateLossEXT2 simula la pérdida en un sistema de archivos EXT2
func simulateLossEXT2(partition MountedPartition) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el sistema EXT2
	ext2fs, err := EstructuraExt2P2.ReadEXT2FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT2: %s", err)
	}

	// Verificar que el sistema esté formateado como EXT2
	if ext2fs.SuperBlock.S_magic != EstructuraarchivocarpetaP2.EXT2_MAGIC {
		return fmt.Errorf("la partición no tiene formato EXT2 válido")
	}

	if ext2fs.SuperBlock.S_filesystem_type != EstructuraarchivocarpetaP2.EXT2_FILESYSTEM_TYPE {
		return fmt.Errorf("la partición no es un sistema de archivos EXT2")
	}

	fmt.Printf("1. Limpiando bitmap de inodos EXT2...\n")
	err = clearBitmapInodes(file, ext2fs.SuperBlock.S_bm_inode_start, ext2fs.SuperBlock.S_inodes_count)
	if err != nil {
		return fmt.Errorf("error limpiando bitmap de inodos: %s", err)
	}
	fmt.Printf("   ✓ Bitmap de inodos limpiado (%d inodos)\n", ext2fs.SuperBlock.S_inodes_count)

	fmt.Printf("2. Limpiando bitmap de bloques EXT2...\n")
	err = clearBitmapBlocks(file, ext2fs.SuperBlock.S_bm_block_start, ext2fs.SuperBlock.S_blocks_count)
	if err != nil {
		return fmt.Errorf("error limpiando bitmap de bloques: %s", err)
	}
	fmt.Printf("   ✓ Bitmap de bloques limpiado (%d bloques)\n", ext2fs.SuperBlock.S_blocks_count)

	fmt.Printf("3. Limpiando área de inodos EXT2...\n")
	err = clearInodeArea(file, ext2fs.SuperBlock.S_inode_start, ext2fs.SuperBlock.S_inodes_count, ext2fs.SuperBlock.S_inode_s)
	if err != nil {
		return fmt.Errorf("error limpiando área de inodos: %s", err)
	}
	fmt.Printf("   ✓ Área de inodos limpiada (%d inodos, %d bytes cada uno)\n", ext2fs.SuperBlock.S_inodes_count, ext2fs.SuperBlock.S_inode_s)

	fmt.Printf("4. Limpiando área de bloques EXT2...\n")
	err = clearBlockArea(file, ext2fs.SuperBlock.S_block_start, ext2fs.SuperBlock.S_blocks_count, ext2fs.SuperBlock.S_block_s)
	if err != nil {
		return fmt.Errorf("error limpiando área de bloques: %s", err)
	}
	fmt.Printf("   ✓ Área de bloques limpiada (%d bloques, %d bytes cada uno)\n", ext2fs.SuperBlock.S_blocks_count, ext2fs.SuperBlock.S_block_s)

	return nil
}

// simulateLossEXT3 simula la pérdida en un sistema de archivos EXT3
func simulateLossEXT3(partition MountedPartition) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el sistema EXT3
	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT3: %s", err)
	}

	// Verificar que el sistema esté formateado como EXT3
	if ext3fs.SuperBlock.S_magic != EstructuraarchivocarpetaP2.EXT2_MAGIC {
		return fmt.Errorf("la partición no tiene formato EXT3 válido")
	}

	if ext3fs.SuperBlock.S_filesystem_type != EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE {
		return fmt.Errorf("la partición no es un sistema de archivos EXT3")
	}

	fmt.Printf("1. Limpiando bitmap de inodos EXT3...\n")
	err = clearBitmapInodes(file, ext3fs.SuperBlock.S_bm_inode_start, ext3fs.SuperBlock.S_inodes_count)
	if err != nil {
		return fmt.Errorf("error limpiando bitmap de inodos: %s", err)
	}
	fmt.Printf("   ✓ Bitmap de inodos limpiado (%d inodos)\n", ext3fs.SuperBlock.S_inodes_count)

	fmt.Printf("2. Limpiando bitmap de bloques EXT3...\n")
	err = clearBitmapBlocks(file, ext3fs.SuperBlock.S_bm_block_start, ext3fs.SuperBlock.S_blocks_count)
	if err != nil {
		return fmt.Errorf("error limpiando bitmap de bloques: %s", err)
	}
	fmt.Printf("   ✓ Bitmap de bloques limpiado (%d bloques)\n", ext3fs.SuperBlock.S_blocks_count)

	fmt.Printf("3. Limpiando área de inodos EXT3...\n")
	err = clearInodeArea(file, ext3fs.SuperBlock.S_inode_start, ext3fs.SuperBlock.S_inodes_count, ext3fs.SuperBlock.S_inode_s)
	if err != nil {
		return fmt.Errorf("error limpiando área de inodos: %s", err)
	}
	fmt.Printf("   ✓ Área de inodos limpiada (%d inodos, %d bytes cada uno)\n", ext3fs.SuperBlock.S_inodes_count, ext3fs.SuperBlock.S_inode_s)

	fmt.Printf("4. Limpiando área de bloques EXT3...\n")
	err = clearBlockArea(file, ext3fs.SuperBlock.S_block_start, ext3fs.SuperBlock.S_blocks_count, ext3fs.SuperBlock.S_block_s)
	if err != nil {
		return fmt.Errorf("error limpiando área de bloques: %s", err)
	}
	fmt.Printf("   ✓ Área de bloques limpiada (%d bloques, %d bytes cada uno)\n", ext3fs.SuperBlock.S_blocks_count, ext3fs.SuperBlock.S_block_s)
	/*
		fmt.Printf("5. Limpiando área de journaling EXT3...\n")
		err = clearJournalingArea(file, ext3fs)
		if err != nil {
			return fmt.Errorf("error limpiando área de journaling: %s", err)
		}
		fmt.Printf("   ✓ Área de journaling limpiada\n")
	*/
	return nil
}

// clearBitmapInodes limpia el bitmap de inodos con \0
func clearBitmapInodes(file *os.File, bitmapStart int32, inodesCount int32) error {
	// Calcular el tamaño del bitmap en bytes
	bitmapSize := (inodesCount + 7) / 8 // Redondear hacia arriba

	// Crear un buffer lleno de ceros
	zeroBuffer := make([]byte, bitmapSize)

	// Escribir el buffer de ceros en el bitmap
	_, err := file.Seek(int64(bitmapStart), 0)
	if err != nil {
		return fmt.Errorf("error posicionándose en bitmap de inodos: %s", err)
	}

	_, err = file.Write(zeroBuffer)
	if err != nil {
		return fmt.Errorf("error escribiendo ceros en bitmap de inodos: %s", err)
	}

	return nil
}

// clearBitmapBlocks limpia el bitmap de bloques con \0
func clearBitmapBlocks(file *os.File, bitmapStart int32, blocksCount int32) error {
	// Calcular el tamaño del bitmap en bytes
	bitmapSize := (blocksCount + 7) / 8 // Redondear hacia arriba

	// Crear un buffer lleno de ceros
	zeroBuffer := make([]byte, bitmapSize)

	// Escribir el buffer de ceros en el bitmap
	_, err := file.Seek(int64(bitmapStart), 0)
	if err != nil {
		return fmt.Errorf("error posicionándose en bitmap de bloques: %s", err)
	}

	_, err = file.Write(zeroBuffer)
	if err != nil {
		return fmt.Errorf("error escribiendo ceros en bitmap de bloques: %s", err)
	}

	return nil
}

// clearInodeArea limpia toda el área de inodos con \0
func clearInodeArea(file *os.File, inodeStart int32, inodesCount int32, inodeSize int32) error {
	// Calcular el tamaño total del área de inodos
	totalSize := inodesCount * inodeSize

	// Crear un buffer lleno de ceros
	zeroBuffer := make([]byte, totalSize)

	// Escribir el buffer de ceros en el área de inodos
	_, err := file.Seek(int64(inodeStart), 0)
	if err != nil {
		return fmt.Errorf("error posicionándose en área de inodos: %s", err)
	}

	_, err = file.Write(zeroBuffer)
	if err != nil {
		return fmt.Errorf("error escribiendo ceros en área de inodos: %s", err)
	}

	return nil
}

// clearBlockArea limpia toda el área de bloques con \0
func clearBlockArea(file *os.File, blockStart int32, blocksCount int32, blockSize int32) error {
	// Calcular el tamaño total del área de bloques
	totalSize := blocksCount * blockSize

	// Crear un buffer lleno de ceros
	zeroBuffer := make([]byte, totalSize)

	// Escribir el buffer de ceros en el área de bloques
	_, err := file.Seek(int64(blockStart), 0)
	if err != nil {
		return fmt.Errorf("error posicionándose en área de bloques: %s", err)
	}

	_, err = file.Write(zeroBuffer)
	if err != nil {
		return fmt.Errorf("error escribiendo ceros en área de bloques: %s", err)
	}

	return nil
}

// clearJournalingArea limpia el área de journaling específica de EXT3
func clearJournalingArea(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem) error {
	// Calcular el inicio y tamaño del área de journaling
	journalingStart := ext3fs.StartOffset + 86              // Después del superbloque (86 bytes)
	journalingSize := ext3fs.SuperBlock.S_inodes_count * 50 // 50 bytes por entrada de journal

	// Crear un buffer lleno de ceros
	zeroBuffer := make([]byte, journalingSize)

	// Escribir el buffer de ceros en el área de journaling
	_, err := file.Seek(int64(journalingStart), 0)
	if err != nil {
		return fmt.Errorf("error posicionándose en área de journaling: %s", err)
	}

	_, err = file.Write(zeroBuffer)
	if err != nil {
		return fmt.Errorf("error escribiendo ceros en área de journaling: %s", err)
	}

	return nil
}
