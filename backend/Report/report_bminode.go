package reportP2

import (
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	"fmt"
	"os"
	"strings"
)

// GenerarBMInodeReporte genera el reporte del bitmap de inodos
func GenerarBMInodeReporte(diskPath string, partitionStart int32, outputPath string) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(diskPath, os.O_RDONLY, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", diskPath, err)
	}
	defer file.Close()

	// Leer el sistema de archivos EXT2
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(diskPath, partitionStart)
	if err != nil {
		return fmt.Errorf("error leyendo el sistema de archivos EXT2: %s", err)
	}

	// Verificar que sea un sistema de archivos EXT2 válido
	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("el sistema de archivos no es EXT2 válido (magic number incorrecto)")
	}

	// Leer el bitmap de inodos
	bitmapInodeSize := (ext2fs.SuperBlock.S_inodes_count + 7) / 8
	bitmapInodeData := make([]byte, bitmapInodeSize)

	_, err = file.Seek(int64(ext2fs.SuperBlock.S_bm_inode_start), 0)
	if err != nil {
		return fmt.Errorf("error posicionándose en el bitmap de inodos: %s", err)
	}

	_, err = file.Read(bitmapInodeData)
	if err != nil {
		return fmt.Errorf("error leyendo el bitmap de inodos: %s", err)
	}

	// Generar el contenido del reporte
	var report strings.Builder
	count := 0

	for inodeNum := int32(0); inodeNum < ext2fs.SuperBlock.S_inodes_count; inodeNum++ {
		bytePos := inodeNum / 8
		bitPos := inodeNum % 8

		if bytePos < int32(len(bitmapInodeData)) {
			if (bitmapInodeData[bytePos] & (1 << (7 - bitPos))) != 0 {
				report.WriteString("1") // Inodo utilizado
			} else {
				report.WriteString("0") // Inodo libre
			}
			count++

			// Insertar un salto de línea cada 20 registros
			if count%20 == 0 {
				report.WriteString("\n")
			}
		}
	}

	// Escribir el reporte en el archivo de salida
	err = os.WriteFile(outputPath, []byte(report.String()), 0644)
	if err != nil {
		return fmt.Errorf("error escribiendo el archivo de reporte: %s", err)
	}

	fmt.Printf("Reporte bm_inode generado exitosamente: %s\n", outputPath)
	return nil
}
