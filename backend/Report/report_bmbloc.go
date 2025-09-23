package reportP2

import (
	estructuraext2P2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	"fmt"
	"os"
	"strings"
)

// ...existing code...
func GenerarBMIBlocReporte(diskPath string, partitionStart int32, outputPath string) error {
	file, err := os.OpenFile(diskPath, os.O_RDONLY, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", diskPath, err)
	}
	defer file.Close()

	ext2fs, err := estructuraext2P2.ReadEXT2FileSystem(diskPath, partitionStart)
	if err != nil {
		return fmt.Errorf("Error leyendo el sistema de archivos EXT2: %s", err)
	}

	if ext2fs.SuperBlock.S_magic != EstructuraarchivocarpetaP2.EXT2_MAGIC {
		return fmt.Errorf("el sistema de archivos no es EXT2 válido (magic number incorrecto)")
	}

	bitmapBloqueSize := (ext2fs.SuperBlock.S_blocks_count + 7) / 8
	bitmapBlockData := make([]byte, bitmapBloqueSize)

	_, err = file.Seek(int64(ext2fs.SuperBlock.S_bm_block_start), 0)
	if err != nil {
		return fmt.Errorf("error posicionándose en el bitmap de bloques: %s", err)
	}

	_, err = file.Read(bitmapBlockData)
	if err != nil {
		return fmt.Errorf("error leyendo el bitmap de bloques: %s", err)
	}

	var report strings.Builder
	count := 0
	bitsPerLine := 20 // <- escribir 20 bits por línea

	for blockNum := int32(0); blockNum < ext2fs.SuperBlock.S_blocks_count; blockNum++ {
		bytePos := blockNum / 8
		bitPos := blockNum % 8

		if bytePos < int32(len(bitmapBlockData)) {
			if (bitmapBlockData[bytePos] & (1 << (7 - bitPos))) != 0 {
				report.WriteByte('1') // Bloque utilizado
			} else {
				report.WriteByte('0') // Bloque libre
			}
			count++
			if count%bitsPerLine == 0 {
				report.WriteString("\n")
			}
		}
	}

	// si quedó una línea incompleta, terminarla con newline
	if count%bitsPerLine != 0 {
		report.WriteString("\n")
	}

	err = os.WriteFile(outputPath, []byte(report.String()), 0644)
	if err != nil {
		return fmt.Errorf("error escribiendo el archivo de salida '%s': %s", outputPath, err)
	}

	fmt.Printf("reporte generado en '%s'\n", outputPath)
	return nil
}
