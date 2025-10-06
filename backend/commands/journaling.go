package commandsP2

import (
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	sessionP2 "MIA_2S2025_P2_202307820/Session"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// JournalEntry representa una entrada del journal para el frontend
type JournalEntry struct {
    Operation string `json:"operation"`
    Path      string `json:"path"`
    Content   string `json:"content"`
    Date      string `json:"date"`
    Count     int32  `json:"count"`
}

// JournalingResponse representa la respuesta del comando journaling
type JournalingResponse struct {
    PartitionID string         `json:"partition_id"`
    FileSystem  string         `json:"filesystem"`
    Entries     []JournalEntry `json:"entries"`
    TotalCount  int            `json:"total_count"`
    Success     bool           `json:"success"`
    Message     string         `json:"message"`
}

// JOURNALING implementa el comando para mostrar las transacciones del journal
func JOURNALING(args []string) {
    // Parsear parámetros
    params := utilitiesP2.ParseParameters(args)

    // Validar parámetro obligatorio -id
    partitionID, exists := params["id"]
    if !exists || strings.TrimSpace(partitionID) == "" {
        fmt.Println("Error: Debe especificar el parámetro obligatorio -id")
        fmt.Println("Uso: journaling -id=<partition_id>")
        fmt.Println("Ejemplo: journaling -id=061Disco1")
        return
    }

    partitionID = strings.ToUpper(strings.Trim(partitionID, "\""))

    // Verificar que hay una sesión activa
    if !sessionP2.IsSessionActive() {
        fmt.Println("Error: Debe iniciar sesión para usar este comando")
        fmt.Println("Use: login -user=<usuario> -pass=<contraseña> -id=<partition_id>")
        return
    }

    // Verificar que la partición esté montada
    mountedPartition, exists := GetMountedPartition(partitionID)
    if !exists {
        fmt.Printf("Error: La partición con ID '%s' no está montada\n", partitionID)
        fmt.Println("Use el comando 'mount' para montar la partición antes de ver el journaling")
        return
    }

    // Detectar el tipo de sistema de archivos
    fsType, err := detectFileSystemTypeForJournaling(mountedPartition)
    if err != nil {
        fmt.Printf("Error detectando el sistema de archivos: %s\n", err)
        return
    }

    // Solo EXT3 tiene journaling
    if fsType != 3 { // EXT3_FILESYSTEM_TYPE
        fmt.Printf("Error: El journaling solo está disponible para sistemas de archivos EXT3\n")
        fmt.Printf("Sistema de archivos detectado: %s\n", getFileSystemTypeName(int32(fsType)))
        return
    }

    fmt.Printf("=== JOURNALING PARA PARTICIÓN %s ===\n", partitionID)
    fmt.Printf("Sistema de archivos: EXT3\n")
    fmt.Printf("Mostrando transacciones del journal...\n\n")

    // Leer y mostrar las entradas del journal
    entries, err := readJournalEntries(mountedPartition)
    if err != nil {
        fmt.Printf("Error leyendo entradas del journal: %s\n", err)
        return
    }

    if len(entries) == 0 {
        fmt.Println("No hay entradas en el journal para mostrar")
        return
    }

    // Mostrar las entradas en formato tabla
    displayJournalEntries(entries)

    fmt.Printf("\nTotal de transacciones: %d\n", len(entries))
    fmt.Printf("Hora de consulta: %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

// detectFileSystemTypeForJournaling detecta el tipo de sistema de archivos
func detectFileSystemTypeForJournaling(partition MountedPartition) (int, error) {
    file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
    if err != nil {
        return -1, fmt.Errorf("error abriendo disco: %s", err)
    }
    defer file.Close()

    // Leer el superbloque
    superBlock := &EstructuraarchivocarpetaP2.SuperBloque{}
    err = utilitiesP2.ReadStruct(file, int64(partition.Start), superBlock)
    if err != nil {
        return -1, fmt.Errorf("error leyendo superbloque: %s", err)
    }

    // Verificar el número mágico
    if superBlock.S_magic != 0xEF53 {
        return -1, fmt.Errorf("número mágico inválido: 0x%X", superBlock.S_magic)
    }

    return int(superBlock.S_filesystem_type), nil
}

// readJournalEntries lee todas las entradas del journal de una partición EXT3
func readJournalEntries(partition MountedPartition) ([]JournalEntry, error) {
    // Leer el sistema EXT3
    ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, partition.Start)
    if err != nil {
        return nil, fmt.Errorf("error leyendo sistema EXT3: %s", err)
    }

    // Obtener las entradas del journal
    journalEntries, err := ext3fs.GetJournalEntries()
    if err != nil {
        return nil, fmt.Errorf("error obteniendo entradas del journal: %s", err)
    }

    // Convertir a formato para mostrar
    var entries []JournalEntry
    for _, entry := range journalEntries {
        // Limpiar strings eliminando caracteres nulos
        operation := strings.TrimRight(string(entry.J_Content.I_Operation[:]), "\x00")
        path := strings.TrimRight(string(entry.J_Content.I_Path[:]), "\x00")
        content := strings.TrimRight(string(entry.J_Content.I_Content[:]), "\x00")

        // Convertir timestamp a fecha legible
        date := time.Unix(int64(entry.J_Content.I_Date), 0).Format("02/01/2006 15:04")

        entries = append(entries, JournalEntry{
            Operation: operation,
            Path:      path,
            Content:   content,
            Date:      date,
            Count:     entry.J_Count,
        })
    }

    return entries, nil
}

// displayJournalEntries muestra las entradas del journal en formato tabla
func displayJournalEntries(entries []JournalEntry) {
    // Encabezado de la tabla
    fmt.Printf("┌─────────┬─────────────────┬──────────────────────────────────────────────┬─────────────────┐\n")
    fmt.Printf("│ %-7s │ %-15s │ %-44s │ %-15s │\n", "Operación", "Path", "Contenido", "Fecha")
    fmt.Printf("├─────────┼─────────────────┼──────────────────────────────────────────────┼─────────────────┤\n")

    // Datos de la tabla
    for _, entry := range entries {
        operation := truncateString(entry.Operation, 7)
        path := truncateString(entry.Path, 15)
        content := truncateString(entry.Content, 44)
        
        fmt.Printf("│ %-7s │ %-15s │ %-44s │ %-15s │\n", 
            operation, path, content, entry.Date)
    }

    // Footer de la tabla
    fmt.Printf("└─────────┴─────────────────┴──────────────────────────────────────────────┴─────────────────┘\n")
}

// truncateString trunca una cadena si es muy larga
func truncateString(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    if maxLen <= 3 {
        return s[:maxLen]
    }
    return s[:maxLen-3] + "..."
}

// HandleJournalingAPI maneja las peticiones API para el journaling
func HandleJournalingAPI(w http.ResponseWriter, r *http.Request) {
    // Permitir CORS
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
    w.Header().Set("Content-Type", "application/json")

    if r.Method == "OPTIONS" {
        return
    }

    if r.Method != "GET" {
        http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
        return
    }

    // Obtener el ID de partición de los query parameters
    partitionID := r.URL.Query().Get("id")
    if partitionID == "" {
        response := JournalingResponse{
            Success: false,
            Message: "Parámetro 'id' es requerido",
        }
        json.NewEncoder(w).Encode(response)
        return
    }

    partitionID = strings.ToUpper(partitionID)

    // Verificar que la partición esté montada
    mountedPartition, exists := GetMountedPartition(partitionID)
    if !exists {
        response := JournalingResponse{
            PartitionID: partitionID,
            Success:     false,
            Message:     fmt.Sprintf("La partición con ID '%s' no está montada", partitionID),
        }
        json.NewEncoder(w).Encode(response)
        return
    }

    // Detectar el tipo de sistema de archivos
    fsType, err := detectFileSystemTypeForJournaling(mountedPartition)
    if err != nil {
        response := JournalingResponse{
            PartitionID: partitionID,
            Success:     false,
            Message:     fmt.Sprintf("Error detectando sistema de archivos: %s", err.Error()),
        }
        json.NewEncoder(w).Encode(response)
        return
    }

    // Solo EXT3 tiene journaling
    if fsType != 3 {
        response := JournalingResponse{
            PartitionID: partitionID,
            FileSystem:  getFileSystemTypeName(int32(fsType)),
            Success:     false,
            Message:     "El journaling solo está disponible para sistemas de archivos EXT3",
        }
        json.NewEncoder(w).Encode(response)
        return
    }

    // Leer las entradas del journal
    entries, err := readJournalEntries(mountedPartition)
    if err != nil {
        response := JournalingResponse{
            PartitionID: partitionID,
            FileSystem:  "EXT3",
            Success:     false,
            Message:     fmt.Sprintf("Error leyendo entradas del journal: %s", err.Error()),
        }
        json.NewEncoder(w).Encode(response)
        return
    }

    // Respuesta exitosa
    response := JournalingResponse{
        PartitionID: partitionID,
        FileSystem:  "EXT3",
        Entries:     entries,
        TotalCount:  len(entries),
        Success:     true,
        Message:     "Journaling cargado exitosamente",
    }

    json.NewEncoder(w).Encode(response)
}