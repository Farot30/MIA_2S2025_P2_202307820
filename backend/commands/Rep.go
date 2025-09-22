package commands

import (
	reportdisk "MIA_2S2025_P1_202307820/Report"
	"MIA_2S2025_P1_202307820/structures"
	"MIA_2S2025_P1_202307820/utilities"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

func REP(args []string) {
	// Parsear parámetros
	params := utilities.ParseParameters(args)

	// Validar parámetros requeridos
	required := []string{"name", "path", "id"}
	if err := utilities.ValidateRequiredParams(params, required); err != nil {
		fmt.Printf("Error: %s\n", err)
		fmt.Println("Uso: rep -name=<tipo_reporte> -path=<ruta_destino> -id=<partition_id> [-path_file_ls=<ruta>]")
		return
	}

	// Obtener parámetros
	name := strings.ToLower(strings.Trim(params["name"], "\""))
	path := strings.Trim(params["path"], "\"")
	id := strings.ToUpper(params["id"])
	pathFileLs := strings.Trim(params["path_file_ls"], "\"")

	// Validar que path no esté vacío (ya debería estar validado por ValidateRequiredParams, pero por seguridad)
	if path == "" {
		fmt.Println("Error: El parámetro path es obligatorio")
		return
	}

	// Validar que name tenga un valor válido
	validReportTypes := map[string]bool{
		"mbr": true, "disk": true, "inode": true, "block": true,
		"bm_inode": true, "bm_block": true, "tree": true,
		"sb": true, "file": true, "ls": true,
	}

	if !validReportTypes[name] {
		fmt.Println("Error: Tipo de reporte inválido")
		fmt.Println("Valores permitidos: mbr, disk, inode, block, bm_inode, bm_block, tree, sb, file, ls")
		return
	}

	// Verificar que la partición existe
	partition, exists := GetMountedPartition(id)
	if !exists {
		fmt.Printf("Error: La partición con ID '%s' no está montada\n", id)
		return
	}

	// Crear directorio de reportes si no existe
	reportDir := "./reportes"
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		fmt.Printf("Error creando directorio 'reportes': %s\n", err)
		return
	}

	if !strings.Contains(path, "/") {
		path = filepath.Join(reportDir, path)
	} else {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("Error creando directorio para el reporte: %s\n", err)
			return
		}
	}

	if filepath.Ext(path) == "" {
		if name == "file" || name == "bm_inode" || name == "bm_block" {
			fmt.Printf("Asignando extensión .txt al reporte: %s\n", name)
			path = path + ".txt"
		} else {
			fmt.Printf("Asignando extensión .png al reporte: %s\n", name)
			path = path + ".png"
		}
	}
	fmt.Printf("Generando reporte en: %s\n", path)

	// Verificar que Graphviz esté instalado
	if !isGraphvizInstalled() {
		fmt.Println("Error: Graphviz no está instalado. Por favor instálelo con:")
		fmt.Println("sudo apt-get install graphviz")
		return
	}

	// Si el usuario no especificó una ruta completa, generarla en la carpeta reportes
	if path == "" || !strings.Contains(path, "/") {
		// Generar nombre basado en tipo de reporte y timestamp
		timestamp := time.Now().Format("20060102_150405")
		ext := ".png"

		if strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".jpeg") {
			ext = ".jpg"
		} else if strings.HasSuffix(path, ".pdf") {
			ext = ".pdf"
		} else if strings.HasSuffix(path, ".svg") {
			ext = ".svg"
		}

		fileName := fmt.Sprintf("%s_%s_%s%s", name, id, timestamp, ext)
		path = filepath.Join(reportDir, fileName)
		fmt.Printf("Guardando reporte en: %s\n", path)
	}

	// Generar reporte según el tipo
	var err error
	switch name {
	case "mbr":
		err = GenerateMBRReport(partition, path)
	case "disk":
		err = GenerateDiskReport(partition, path)
	case "inode":
		err = GenerateInodeReport(partition, path)
	case "block":
		err = GenerateBlockReport(partition, path)
	case "bm_inode":
		err = GenerateBitmapInodeReport(partition, path)
	case "bm_block":
		err = GenerateBitmapBlockReport(partition, path)
	case "tree":
		err = GenerateTreeReport(partition, path)
	case "sb":
		err = GenerateSuperblockReport(partition, path)
	case "file":
		if pathFileLs == "" {
			fmt.Println("Error: El reporte file requiere el parámetro -path_file_ls")
			return
		}
		err = GenerateFileReport(partition, path, pathFileLs)
	case "ls":

		if pathFileLs == "" {
			fmt.Println("Error: El reporte ls requiere el parámetro -path_file_ls")
			return
		}
		err = GenerateLsReport(partition, path, pathFileLs)
	}

	if err != nil {
		fmt.Printf("Error generando reporte '%s': %s\n", name, err)
	} else {

		fmt.Printf("Reporte '%s' generado exitosamente:\n", name)
	}
}

// isGraphvizInstalled verifica si Graphviz está instalado
func isGraphvizInstalled() bool {
	cmd := exec.Command("dot", "-V")
	return cmd.Run() == nil
}

// GenerateMBRReport genera un reporte del MBR y EBRs del disco
func GenerateMBRReport(partition MountedPartition, outputPath string) error {
	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el MBR del disco
	var mbr structures.MBR
	if err := utilities.ReadStruct(file, 0, &mbr); err != nil {
		return fmt.Errorf("error leyendo MBR: %s", err)
	}

	// Generar código DOT para Graphviz
	var dot strings.Builder

	dot.WriteString("digraph MBR {\n")
	dot.WriteString("  node [shape=plaintext];\n")
	dot.WriteString("  rankdir=LR;\n")
	dot.WriteString("  MBR [label=<\n")
	dot.WriteString("    <table border='1' cellborder='1' cellspacing='0'>\n")
	dot.WriteString("      <tr><td colspan='2' bgcolor='purple'><font color='white'><b>REPORTE DE MBR</b></font></td></tr>\n")

	// Información del MBR
	dot.WriteString(fmt.Sprintf("      <tr><td>mbr_tamaño</td><td>%d</td></tr>\n", mbr.MbrTamano))
	dot.WriteString(fmt.Sprintf("      <tr><td bgcolor='#E6E6FA'>mbr_fecha_creacion</td><td bgcolor='#E6E6FA'>%s</td></tr>\n",
		strings.TrimRight(string(mbr.MbrFechaCreacion[:]), "\x00")))
	dot.WriteString(fmt.Sprintf("      <tr><td>mbr_disk_signature</td><td>%d</td></tr>\n", mbr.MbrDskSignature))

	// Información de cada partición con colores alternados
	for _, partition := range mbr.MbrPartitions {
		if partition.PartS > 0 { // Partición en uso
			dot.WriteString(fmt.Sprintf("      <tr><td colspan='2' bgcolor='#4B0082'><font color='white'><b>Particion</b></font></td></tr>\n"))
			var statusText string
			switch {
			case partition.PartStatus == 0 || partition.PartStatus == '0': // Valor numérico 0 o caracter ASCII '0'
				statusText = "0 (No montada)"
			case partition.PartStatus == 1 || partition.PartStatus == '1': // Valor numérico 1 o caracter ASCII '1'
				statusText = "1 (Montada)"
			case partition.PartStatus == 2 || partition.PartStatus == '2': // Valor numérico 2 o caracter ASCII '2'
				statusText = "2 (Montada)" // Aparentemente también indica montada
			default:
				// Para depuración, mostremos el valor exacto:
				statusText = fmt.Sprintf("%d (0x%X)", partition.PartStatus, partition.PartStatus)
			}
			dot.WriteString(fmt.Sprintf("      <tr><td>part_status</td><td>%s</td></tr>\n", statusText))
			dot.WriteString(fmt.Sprintf("      <tr><td bgcolor='#E6E6FA'>part_type</td><td bgcolor='#E6E6FA'>%c</td></tr>\n", partition.PartType))
			dot.WriteString(fmt.Sprintf("      <tr><td>part_fit</td><td>%c</td></tr>\n", partition.PartFit))
			dot.WriteString(fmt.Sprintf("      <tr><td bgcolor='#E6E6FA'>part_start</td><td bgcolor='#E6E6FA'>%d</td></tr>\n", partition.PartStart))
			dot.WriteString(fmt.Sprintf("      <tr><td>part_size</td><td>%d</td></tr>\n", partition.PartS))
			dot.WriteString(fmt.Sprintf("      <tr><td bgcolor='#E6E6FA'>part_name</td><td bgcolor='#E6E6FA'>%s</td></tr>\n",
				strings.TrimRight(string(partition.PartName[:]), "\x00")))
		}
	}

	dot.WriteString("    </table>\n")
	dot.WriteString("  >];\n")

	// Agregar EBRs si existen particiones extendidas
	for _, partition := range mbr.MbrPartitions {
		if partition.PartType == 'E' && partition.PartS > 0 {
			// Encontrar la cadena de EBRs
			GenerateEBRsReport(&dot, file, partition.PartStart, partition.PartStart+partition.PartS)
			break
		}
	}

	dot.WriteString("}\n")

	// Escribir el archivo DOT
	dotFile := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".dot"
	if err := os.WriteFile(dotFile, []byte(dot.String()), 0644); err != nil {
		return fmt.Errorf("error escribiendo archivo DOT: %s", err)
	}

	// Generar la imagen con Graphviz
	ext := filepath.Ext(outputPath)
	if ext == "" {
		outputPath += ".png"
		ext = ".png"
	}

	// Eliminar el punto del ext
	format := strings.TrimPrefix(ext, ".")

	cmd := exec.Command("dot", "-T"+format, dotFile, "-o", outputPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error generando imagen con Graphviz: %s", err)
	}

	return nil
}

// GenerateEBRsReport genera el reporte de los EBRs en una partición extendida
func GenerateEBRsReport(dot *strings.Builder, file *os.File, startPos int32, endPos int32) {
	currentEBRPos := startPos
	ebrCount := 0

	for currentEBRPos != -1 && currentEBRPos < endPos {
		// Leer el EBR
		ebr := structures.NewEBR()
		if _, err := file.Seek(int64(currentEBRPos), 0); err != nil {
			break
		}

		ebrData := make([]byte, int(unsafe.Sizeof(*ebr)))
		if _, err := file.Read(ebrData); err != nil {
			break
		}

		if err := ebr.FromBytes(ebrData); err != nil {
			break
		}

		// Solo mostrar EBRs que tengan una partición lógica asociada
		if ebr.PartS > 0 {
			ebrCount++

			dot.WriteString(fmt.Sprintf("  EBR%d [label=<\n", ebrCount))
			dot.WriteString("    <table border='1' cellborder='1' cellspacing='0'>\n")
			dot.WriteString(fmt.Sprintf("      <tr><td colspan='2' bgcolor='#FF9980'><font color='black'><b>Particion Logica</b></font></td></tr>\n"))
			var ebrStatusText string
			switch ebr.PartMount {
			case '0', 0: // Valor numérico 0 o caracter ASCII '0'
				ebrStatusText = "0 (No montada)"
			case '1', 1: // Valor numérico 1 o caracter ASCII '1'
				ebrStatusText = "1 (Montada)"
			default:
				ebrStatusText = fmt.Sprintf("%d (Desconocido)", ebr.PartMount)
			}
			dot.WriteString(fmt.Sprintf("      <tr><td>part_status</td><td>%s</td></tr>\n", ebrStatusText))
			dot.WriteString(fmt.Sprintf("      <tr><td bgcolor='#FFD6CC'>part_next</td><td bgcolor='#FFD6CC'>%d</td></tr>\n", ebr.PartNext))
			dot.WriteString(fmt.Sprintf("      <tr><td>part_fit</td><td>%c</td></tr>\n", ebr.PartFit))
			dot.WriteString(fmt.Sprintf("      <tr><td bgcolor='#FFD6CC'>part_start</td><td bgcolor='#FFD6CC'>%d</td></tr>\n", ebr.PartStart))
			dot.WriteString(fmt.Sprintf("      <tr><td>part_size</td><td>%d</td></tr>\n", ebr.PartS))
			dot.WriteString(fmt.Sprintf("      <tr><td bgcolor='#FFD6CC'>part_name</td><td bgcolor='#FFD6CC'>%s</td></tr>\n",
				strings.TrimRight(string(ebr.PartName[:]), "\x00")))
			dot.WriteString("    </table>\n")
			dot.WriteString("  >];\n")

			// Conectar los nodos en secuencia
			if ebrCount > 1 {
				dot.WriteString(fmt.Sprintf("  EBR%d -> EBR%d;\n", ebrCount-1, ebrCount))
			} else {
				dot.WriteString("  MBR -> EBR1;\n")
			}
		}

		// Avanzar al siguiente EBR
		if ebr.PartNext <= 0 {
			break
		}
		currentEBRPos = ebr.PartNext
	}
}

// GenerateDiskReport genera una representación gráfica del disco
func GenerateDiskReport(partition MountedPartition, outputPath string) error {
	return reportdisk.GenerateDiskReport(partition.Path, partition.Start, outputPath)
}

// GenerateInodeReport muestra información de los inodos
func GenerateInodeReport(partition MountedPartition, outputPath string) error {
	return reportdisk.GenerateInodeReport(partition.Path, partition.Start, outputPath)
}

// GenerateBlockReport muestra información de los bloques
func GenerateBlockReport(partition MountedPartition, outputPath string) error {

	return reportdisk.GenerarBloqueReporte(partition.Path, partition.Start, outputPath)
}

// GenerateBitmapInodeReport muestra el bitmap de inodos
func GenerateBitmapInodeReport(partition MountedPartition, outputPath string) error {
	// Implementación pendiente
	return reportdisk.GenerarBMInodeReporte(partition.Path, partition.Start, outputPath)
}

// GenerateBitmapBlockReport muestra el bitmap de bloques
func GenerateBitmapBlockReport(partition MountedPartition, outputPath string) error {
	// Implementación pendiente
	return reportdisk.GenerarBMIBlocReporte(partition.Path, partition.Start, outputPath)
}

// GenerateTreeReport muestra la estructura de directorios como árbol
func GenerateTreeReport(partition MountedPartition, outputPath string) error {
	// Implementación pendiente
	return reportdisk.GenerateTreeReport(partition.Path, partition.Start, outputPath)
}

// GenerateSuperblockReport muestra información del superbloque
func GenerateSuperblockReport(partition MountedPartition, outputPath string) error {
	// Implementación pendiente
	return reportdisk.GenerateSuperblockReport(partition.Path, partition.Start, outputPath)
}

// GenerateFileReport muestra información de un archivo específico
func GenerateFileReport(partition MountedPartition, outputPath string, filePath string) error {
	// Implementación pendiente
	return reportdisk.GenerateFileReport(partition.Path, partition.Start, partition.ID, filePath, outputPath)
}

// GenerateLsReport muestra contenido de un directorio específico
func GenerateLsReport(partition MountedPartition, outputPath string, dirPath string) error {
	// Implementación pendiente
	return reportdisk.GenerateLsReport(partition.Path, partition.Start, dirPath, outputPath)
}
