package commandsP2

import (
	structuresP2 "MIA_2S2025_P2_202307820/structures"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MKDISK implementa el comando para crear discos virtuales
func MKDISK(args []string) {
	// Parsear parámetros
	params := utilitiesP2.ParseParameters(args)

	// Validar parámetros requeridos
	required := []string{"size", "path"}
	if err := utilitiesP2.ValidateRequiredParams(params, required); err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	// Obtener parámetros
	sizeStr := params["size"]
	path := params["path"]
	unit := strings.ToUpper(params["unit"])
	fit := strings.ToUpper(params["fit"])

	// Validar y convertir tamaño
	size, err := strconv.Atoi(sizeStr)
	if err != nil || size <= 0 {
		fmt.Println("Error: El tamaño debe ser un número positivo mayor que cero")
		return
	}

	// Aplicar unidades (por defecto Megabytes)
	var sizeInBytes int32
	switch unit {
	case "K":
		sizeInBytes = int32(size * 1024)
	case "M", "":
		sizeInBytes = int32(size * 1024 * 1024)
	default:
		fmt.Printf("Error: Unidad '%s' no válida. Use K para Kilobytes o M para Megabytes\n", unit)
		return
	}

	// Validar fit (por defecto First Fit)
	var fitByte byte
	switch fit {
	case "BF":
		fitByte = 'B'
	case "FF", "":
		fitByte = 'F'
	case "WF":
		fitByte = 'W'
	default:
		fmt.Printf("Error: Ajuste '%s' no válido. Use BF, FF o WF\n", fit)
		return
	}

	// Cambiar la extensión del archivo a .mia
	if filepath.Ext(path) != ".mia" {
		path = strings.TrimSuffix(path, filepath.Ext(path)) + ".mia"
	}

	// Verificar si el archivo ya existe
	if utilitiesP2.FileExists(path) {
		fmt.Printf("Error: El archivo '%s' ya existe\n", path)
		return
	}

	// Crear directorios si no existen
	if err := utilitiesP2.CreateDirectoryIfNotExists(path); err != nil {
		fmt.Printf("Error creando directorios: %s\n", err)
		return
	}

	// Crear el archivo del disco
	if err := createDiskFile(path, sizeInBytes, fitByte); err != nil {
		fmt.Printf("Error creando el disco: %s\n", err)
		return
	}

	// INFORMACIÓN DETALLADA DE DEPURACIÓN
	fmt.Printf("\n=== DISCO CREADO EXITOSAMENTE ===\n")
	fmt.Printf("Ruta: %s\n", path)
	fmt.Printf("Tamaño: %d bytes (%.2f %s)\n", sizeInBytes,
		float64(size), map[string]string{"K": "KB", "M": "MB", "": "MB"}[strings.ToLower(unit)])
	fmt.Printf("Ajuste: %s\n", map[byte]string{'B': "Best Fit", 'F': "First Fit", 'W': "Worst Fit"}[fitByte])
	fmt.Printf("Máximo particiones: 4 primarias (o 3 primarias + 1 extendida)\n")

	// INFORMACIÓN DEL MBR
	fmt.Printf("\n--- INFORMACIÓN DEL MBR ---\n")
	file, err := os.OpenFile(path, os.O_RDONLY, 0666)
	if err == nil {
		defer file.Close()
		var mbr structuresP2.MBR
		if utilitiesP2.ReadStruct(file, 0, &mbr) == nil {
			fmt.Printf("Signature: %d\n", mbr.MbrDskSignature)
			fmt.Printf("Fecha creación: %s\n", strings.TrimRight(string(mbr.MbrFechaCreacion[:]), "\x00"))
			fmt.Printf("Tamaño MBR: %d bytes\n", mbr.GetSize())
			fmt.Printf("Fit configurado: %c\n", mbr.DskFit)

			// Mostrar estado inicial de particiones
			fmt.Printf("\n--- ESTADO INICIAL PARTICIONES ---\n")
			for i := 0; i < 4; i++ {
				partition := mbr.MbrPartitions[i]
				fmt.Printf("Partición %d: ", i+1)
				if partition.PartS == 0 {
					fmt.Printf("Libre\n")
				} else {
					partName := strings.TrimRight(string(partition.PartName[:]), "\x00")
					fmt.Printf("%s (%c, %d bytes, start: %d)\n",
						partName, partition.PartType, partition.PartS, partition.PartStart)
				}
			}
		}
	}
	fmt.Printf("=====================================\n")
}

// createDiskFile crea el archivo físico del disco y escribe el MBR
func createDiskFile(path string, size int32, fit byte) error {
	// Crear el archivo
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("no se pudo crear el archivo: %v", err)
	}
	defer file.Close()

	// Crear el MBR
	mbr := structuresP2.NewMBR(size, fit)

	// Escribir el MBR al inicio del archivo
	mbrBytes := mbr.ToBytes()
	if _, err := file.Write(mbrBytes); err != nil {
		return fmt.Errorf("error escribiendo MBR: %v", err)
	}

	// Llenar el resto del archivo con ceros usando un buffer de 1024 bytes
	buffer := make([]byte, 1024) // Buffer inicializado con ceros (0x00)
	remaining := int64(size) - int64(len(mbrBytes))

	for remaining > 0 {
		writeSize := int64(len(buffer))
		if remaining < writeSize {
			writeSize = remaining
		}

		// Escribir el buffer al archivo
		if _, err := file.Write(buffer[:writeSize]); err != nil {
			return fmt.Errorf("error llenando el disco: %v", err)
		}

		remaining -= writeSize
	}

	return nil
}
