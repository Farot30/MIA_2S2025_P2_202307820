package reportP2

import (
	structuresP2 "MIA_2S2025_P2_202307820/structures"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"
)

// DiskSection representa una sección del disco (MBR, partición o espacio libre)
type DiskSection struct {
	Name       string
	Type       string // "MBR", "Primaria", "Extendida", "Lógica", "Libre"
	Start      int32
	Size       int32
	Color      string
	Percentage float64
}

// GenerateDiskReport genera una representación gráfica del disco
func GenerateDiskReport(diskPath string, partitionStart int32, outputPath string) error {
	file, err := os.OpenFile(diskPath, os.O_RDONLY, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", diskPath, err)
	}
	defer file.Close()

	// Leer el MBR del disco
	var mbr structuresP2.MBR
	if err := utilitiesP2.ReadStruct(file, 0, &mbr); err != nil {
		return fmt.Errorf("error leyendo MBR: %s", err)
	}

	// Generar código DOT para Graphviz
	var dot strings.Builder

	fmt.Println("Verificando DOT generado...")
	// Mostrar las primeras líneas para depuración
	dotLines := strings.Split(dot.String(), "\n")
	maxLines := 10
	if len(dotLines) < maxLines {
		maxLines = len(dotLines)
	}
	for i := 0; i < maxLines; i++ {
		fmt.Println(dotLines[i])
	}
	fmt.Println("...")

	dot.WriteString("digraph DISK_REPORT {\n")
	dot.WriteString("  node [shape=plaintext];\n")
	dot.WriteString("  rankdir=TB;\n")

	// Crear el título del disco
	diskName := filepath.Base(diskPath)
	dot.WriteString(fmt.Sprintf("  disk_title [label=\"%s\", fontsize=18, fontcolor=darkblue];\n", diskName))

	// Crear el nodo principal que mostrará el disco
	dot.WriteString("  disk [label=<\n")
	dot.WriteString("    <table border='0' cellborder='1' cellspacing='0' cellpadding='4'>\n")
	dot.WriteString(fmt.Sprintf("      <tr><td colspan='%d' bgcolor='#336699'><font color='white'><b>%s</b></font></td></tr>\n",
		calculateNumColumns(mbr), diskName))

	// Identificar y mostrar todas las secciones del disco
	diskSections := identifyDiskSections(mbr, diskPath)
	generateDiskSectionsHTML(&dot, diskSections, mbr, diskPath)

	dot.WriteString("    </table>\n")
	dot.WriteString("  >];\n")

	// Añadir leyenda con información adicional
	addDiskLegend(&dot, mbr, diskPath)

	// Definir relaciones
	dot.WriteString("  disk_title -> disk;\n")

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

	fmt.Printf("Reporte DISK generado exitosamente: %s\n", outputPath)
	return nil
}

// calculateNumColumns calcula el número de columnas necesarias para la tabla del disco
func calculateNumColumns(mbr structuresP2.MBR) int {
	// Como es solo para calcular el número de columnas, podemos usar una ruta vacía
	// o mejor aún, modificar la función para que sea más simple
	sections := identifyDiskSectionsForCount(mbr)
	return len(sections)
}

func identifyDiskSectionsForCount(mbr structuresP2.MBR) []DiskSection {
	var sections []DiskSection

	// Añadir MBR
	mbrSection := DiskSection{
		Name:  "MBR",
		Type:  "MBR",
		Start: 0,
		Size:  int32(mbr.GetSize()),
	}
	sections = append(sections, mbrSection)

	// Contar particiones válidas
	for _, p := range mbr.MbrPartitions {
		if p.PartS > 0 {
			section := DiskSection{
				Name:  strings.TrimRight(string(p.PartName[:]), "\x00"),
				Type:  string(p.PartType),
				Start: p.PartStart,
				Size:  p.PartS,
			}
			sections = append(sections, section)
		}
	}

	return sections
}

// identifyDiskSections identifica todas las secciones del disco (MBR, particiones y espacios libres)
func identifyDiskSections(mbr structuresP2.MBR, disktPath string) []DiskSection {
	var sections []DiskSection

	// Añadir el MBR
	mbrSection := DiskSection{
		Name:  "MBR",
		Type:  "MBR",
		Start: 0,
		Size:  int32(mbr.GetSize()),
		Color: "#003366",
	}
	sections = append(sections, mbrSection)

	// Recopilar todas las particiones válidas
	var partitions []structuresP2.Partition
	for _, p := range mbr.MbrPartitions {
		if p.PartS > 0 {
			partitions = append(partitions, p)
		}
	}

	// Ordenar las particiones por posición de inicio
	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].PartStart < partitions[j].PartStart
	})

	// Identificar espacios libres y añadir todas las secciones en orden
	lastEnd := mbrSection.Size

	for _, p := range partitions {
		// Si hay espacio entre la última sección y esta partición, añadirlo como espacio libre
		if p.PartStart > lastEnd {
			freeSection := DiskSection{
				Name:  "Libre",
				Type:  "Libre",
				Start: lastEnd,
				Size:  p.PartStart - lastEnd,
				Color: "#CCCCCC",
			}
			sections = append(sections, freeSection)
		}

		// Añadir la partición actual
		var sectionType, sectionColor string

		switch p.PartType {
		case 'P':
			sectionType = "Primaria"
			sectionColor = "#66AACC"
		case 'E':
			sectionType = "Extendida"
			sectionColor = "#6699AA"

			// Crear la sección de la partición extendida
			extendedSection := DiskSection{
				Name:  strings.TrimRight(string(p.PartName[:]), "\x00"),
				Type:  sectionType,
				Start: p.PartStart,
				Size:  p.PartS,
				Color: sectionColor,
			}
			sections = append(sections, extendedSection)

			// Obtener y procesar las secciones lógicas dentro de la extendida
			_ = identifyLogicalSections(disktPath, p, mbr.MbrTamano)

			// Agregar un nodo separado para mostrar el contenido de la partición extendida
			// (Esto lo manejaremos en la generación HTML)

			lastEnd = p.PartStart + p.PartS
			continue
		}

		section := DiskSection{
			Name:  strings.TrimRight(string(p.PartName[:]), "\x00"),
			Type:  sectionType,
			Start: p.PartStart,
			Size:  p.PartS,
			Color: sectionColor,
		}
		sections = append(sections, section)

		lastEnd = p.PartStart + p.PartS
	}

	// Si hay espacio libre al final, añadirlo
	if lastEnd < mbr.MbrTamano {
		freeSection := DiskSection{
			Name:  "Libre",
			Type:  "Libre",
			Start: lastEnd,
			Size:  mbr.MbrTamano - lastEnd,
			Color: "#CCCCCC",
		}
		sections = append(sections, freeSection)
	}

	// Calcular porcentajes
	for i := range sections {
		sections[i].Percentage = float64(sections[i].Size) / float64(mbr.MbrTamano) * 100.0
	}

	return sections
}

func identifyLogicalSections(diskPath string, extendedPartition structuresP2.Partition, diskSize int32) []DiskSection {
	var sections []DiskSection

	// Abrir el archivo del disco
	file, err := os.OpenFile(diskPath, os.O_RDONLY, 0666)
	if err != nil {
		fmt.Printf("Error abriendo disco para leer EBRs: %s\n", err)
		return sections
	}
	defer file.Close()

	extendedStart := extendedPartition.PartStart
	extendedEnd := extendedStart + extendedPartition.PartS
	currentPos := extendedStart
	lastEnd := extendedStart

	// Leer el primer EBR
	ebr := structuresP2.NewEBR()
	_, err = file.Seek(int64(currentPos), 0)
	if err != nil {
		return sections
	}

	ebrData := make([]byte, int(unsafe.Sizeof(*ebr)))
	if _, err := file.Read(ebrData); err != nil {
		return sections
	}

	if err := ebr.FromBytes(ebrData); err != nil {
		return sections
	}

	// Primer EBR siempre está presente
	ebrSection := DiskSection{
		Name:  "EBR",
		Type:  "EBR",
		Start: currentPos,
		Size:  int32(unsafe.Sizeof(*ebr)),
		Color: "#FF6600", // Naranja para EBRs
	}
	sections = append(sections, ebrSection)

	lastEnd = currentPos + ebrSection.Size

	// Procesar la cadena de EBRs
	for {
		// Si hay una partición lógica activa
		if ebr.PartS > 0 {
			// Verificar si hay espacio entre EBR y partición lógica
			if ebr.PartStart > lastEnd {
				freeSection := DiskSection{
					Name:  "Libre",
					Type:  "Libre",
					Start: lastEnd,
					Size:  ebr.PartStart - lastEnd,
					Color: "#DDDDDD", // Gris claro para espacio libre
				}
				sections = append(sections, freeSection)
			}

			// Añadir la partición lógica
			logicalSection := DiskSection{
				Name:  strings.TrimRight(string(ebr.PartName[:]), "\x00"),
				Type:  "Lógica",
				Start: ebr.PartStart,
				Size:  ebr.PartS,
				Color: "#99CCFF", // Azul claro para particiones lógicas
			}
			sections = append(sections, logicalSection)

			lastEnd = ebr.PartStart + ebr.PartS
		}

		// Si no hay siguiente EBR, salir
		if ebr.PartNext <= 0 || ebr.PartNext >= extendedEnd {
			break
		}

		// Si hay espacio entre la última sección y el siguiente EBR
		if ebr.PartNext > lastEnd {
			freeSection := DiskSection{
				Name:  "Libre",
				Type:  "Libre",
				Start: lastEnd,
				Size:  ebr.PartNext - lastEnd,
				Color: "#DDDDDD",
			}
			sections = append(sections, freeSection)
		}

		// Leer el siguiente EBR
		currentPos = ebr.PartNext

		// Añadir el siguiente EBR
		ebrSection = DiskSection{
			Name:  "EBR",
			Type:  "EBR",
			Start: currentPos,
			Size:  int32(unsafe.Sizeof(*ebr)),
			Color: "#FF6600",
		}
		sections = append(sections, ebrSection)

		_, err = file.Seek(int64(currentPos), 0)
		if err != nil {
			break
		}

		if _, err := file.Read(ebrData); err != nil {
			break
		}

		if err := ebr.FromBytes(ebrData); err != nil {
			break
		}

		lastEnd = currentPos + ebrSection.Size
	}

	// Si hay espacio libre al final de la partición extendida
	if lastEnd < extendedEnd {
		freeSection := DiskSection{
			Name:  "Libre",
			Type:  "Libre",
			Start: lastEnd,
			Size:  extendedEnd - lastEnd,
			Color: "#DDDDDD",
		}
		sections = append(sections, freeSection)
	}

	// Calcular porcentajes relativos a la partición extendida
	for i := range sections {
		// Porcentaje global respecto al disco completo
		sections[i].Percentage = float64(sections[i].Size) / float64(diskSize) * 100.0
	}

	return sections
}

// generateDiskSectionsHTML genera el HTML para las secciones del disco
func generateDiskSectionsHTML(dot *strings.Builder, sections []DiskSection, mbr structuresP2.MBR, diskPath string) {
	// Primera fila: las secciones con sus nombres
	dot.WriteString("      <tr>")
	for _, section := range sections {
		widthPercentage := section.Percentage * 10
		if widthPercentage < 5 {
			widthPercentage = 5
		}

		// Si es una partición extendida, mostrarla con su contenido
		if section.Type == "Extendida" {
			// Encontrar la partición extendida original
			var extendedPartition structuresP2.Partition
			for _, p := range mbr.MbrPartitions {
				if p.PartType == 'E' && p.PartStart == section.Start {
					extendedPartition = p
					break
				}
			}

			// Obtener las secciones lógicas
			logicalSections := identifyLogicalSections(diskPath, extendedPartition, mbr.MbrTamano)

			// Crear una tabla anidada más simple para la partición extendida
			dot.WriteString(fmt.Sprintf("<td width='%.0f' bgcolor='%s'>\n", widthPercentage, section.Color))
			dot.WriteString("            <table border='0' cellborder='1' cellspacing='0' cellpadding='2'>\n")

			// Calcular colspan seguro
			colspan := 1
			if len(logicalSections) > 0 {
				colspan = len(logicalSections)
			}

			// Encabezado de la partición extendida
			dot.WriteString(fmt.Sprintf("              <tr><td colspan='%d'>%s (%s)</td></tr>\n",
				colspan, section.Name, section.Type))

			// Contenido de la partición extendida
			if len(logicalSections) > 0 {
				// Mostrar información de particiones lógicas con anchos proporcionales
				dot.WriteString("              <tr>")
				for _, ls := range logicalSections {
					// Calcular ancho basado en porcentaje (asegurar un mínimo de 5%)
					widthLS := ls.Percentage
					if widthLS < 5 {
						widthLS = 5
					}

					dot.WriteString(fmt.Sprintf("<td width='%.0f%%' bgcolor='%s'>%s</td>",
						widthLS, ls.Color, ls.Name))
				}
				dot.WriteString("</tr>\n")

				// Mostrar tipo de sección
				dot.WriteString("              <tr>")
				for _, ls := range logicalSections {
					dot.WriteString(fmt.Sprintf("<td bgcolor='%s'>%s</td>",
						ls.Color, ls.Type))
				}
				dot.WriteString("</tr>\n")

				// Mostrar porcentajes
				dot.WriteString("              <tr>")
				for _, ls := range logicalSections {
					dot.WriteString(fmt.Sprintf("<td bgcolor='%s'>%.1f%% del Disco</td>",
						ls.Color, ls.Percentage))
				}
				dot.WriteString("</tr>\n")
			} else {
				dot.WriteString("              <tr><td>Sin particiones lógicas</td></tr>\n")
			}

			dot.WriteString("            </table>\n")
			dot.WriteString("          </td>")
		} else {
			// Sección normal (no extendida)
			dot.WriteString(fmt.Sprintf("<td width='%.0f' bgcolor='%s'>%s</td>",
				widthPercentage, section.Color, section.Name))
		}
	}
	dot.WriteString("</tr>\n")

	// Segunda fila: los tipos
	dot.WriteString("      <tr>")
	for _, section := range sections {
		dot.WriteString(fmt.Sprintf("<td bgcolor='%s'>%s</td>", section.Color, section.Type))
	}
	dot.WriteString("</tr>\n")

	// Tercera fila: los porcentajes
	dot.WriteString("      <tr>")
	for _, section := range sections {
		dot.WriteString(fmt.Sprintf("<td>%.1f%% del Disco</td>", section.Percentage))
	}
	dot.WriteString("</tr>\n")
}

// addDiskLegend añade una leyenda con información adicional del disco
func addDiskLegend(dot *strings.Builder, mbr structuresP2.MBR, diskPath string) {
	dot.WriteString("  legend [label=<\n")
	dot.WriteString("    <table border='0' cellborder='1' cellspacing='0' cellpadding='4'>\n")
	dot.WriteString("      <tr><td colspan='2' bgcolor='#336699'><font color='white'><b>Información del Disco</b></font></td></tr>\n")
	dot.WriteString(fmt.Sprintf("      <tr><td>Tamaño</td><td>%d bytes</td></tr>\n", mbr.MbrTamano))
	dot.WriteString(fmt.Sprintf("      <tr><td>Fecha creación</td><td>%s</td></tr>\n",
		strings.TrimRight(string(mbr.MbrFechaCreacion[:]), "\x00")))
	dot.WriteString(fmt.Sprintf("      <tr><td>Signature</td><td>%d</td></tr>\n", mbr.MbrDskSignature))
	dot.WriteString(fmt.Sprintf("      <tr><td>Fit</td><td>%c</td></tr>\n", mbr.DskFit))
	dot.WriteString(fmt.Sprintf("      <tr><td>Ruta</td><td>%s</td></tr>\n", diskPath))

	// Resumen de particiones
	var numPrimary, numExtended int
	for _, p := range mbr.MbrPartitions {
		if p.PartS > 0 {
			if p.PartType == 'P' {
				numPrimary++
			} else if p.PartType == 'E' {
				numExtended++
			}
		}
	}

	dot.WriteString(fmt.Sprintf("      <tr><td>Particiones Primarias</td><td>%d / 4</td></tr>\n", numPrimary))
	dot.WriteString(fmt.Sprintf("      <tr><td>Particiones Extendidas</td><td>%d / 1</td></tr>\n", numExtended))

	dot.WriteString("    </table>\n")
	dot.WriteString("  >];\n")
}
