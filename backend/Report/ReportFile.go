package reportP2

import (
	EstructuraExt2P2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func GenerateFileReport(diskPath string, partitionStart int32, mountID string, filePath string, outputPath string) error {
	ext2fs, err := EstructuraExt2P2.ReadEXT2FileSystem(diskPath, partitionStart)
	if err != nil {
		return fmt.Errorf("leer EXT2: %v", err)
	}

	// <-- Ajusta este llamado según tu API: ReadFile, ReadFileContent, Cat, etc. -->
	content, err := ext2fs.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("leer archivo '%s': %v", filePath, err)
	}

	var sb strings.Builder
	sb.WriteString("Reporte File\n")
	sb.WriteString("-------------------------\n")
	sb.WriteString(fmt.Sprintf("Disco: %s\n", filepath.Base(diskPath)))
	sb.WriteString(fmt.Sprintf("Mount ID: %s\n", mountID))
	sb.WriteString(fmt.Sprintf("Partición inicio (byte): %d\n", partitionStart))
	sb.WriteString(fmt.Sprintf("Ruta archivo: %s\n\n", filePath))
	sb.WriteString("Contenido:\n")
	if content == "" {
		sb.WriteString("(archivo vacío)\n")
	} else {
		sb.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			sb.WriteString("\n")
		}
	}

	if filepath.Ext(outputPath) == "" {
		outputPath = outputPath + ".txt"
	}
	if err := os.WriteFile(outputPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("escribir reporte file: %v", err)
	}

	return nil
}
