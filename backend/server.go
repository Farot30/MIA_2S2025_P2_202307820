package main

import (
	commandsP2 "MIA_2S2025_P2_202307820/commands"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RequestBody estructura para recibir comandos del frontend
type RequestBody struct {
	Commands string `json:"commands"`
}

// ResponseBody estructura para enviar respuestas al frontend
type ResponseBody struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

func captureOutput(fn func()) string {
	// Crear un buffer para capturar la salida
	var buf bytes.Buffer

	// Guardar stdout original y output original del logger
	originalStdout := os.Stdout
	originalLogOutput := log.Writer()

	// Crear un pipe
	r, w, _ := os.Pipe()

	// Reemplazar stdout y logger
	os.Stdout = w
	log.SetOutput(w)

	// Canal para capturar la salida
	outputChan := make(chan string)

	// Goroutine para leer del pipe
	go func() {
		io.Copy(&buf, r)
		outputChan <- buf.String()
	}()

	// Ejecutar la función
	fn()

	// Restaurar stdout y logger y cerrar el pipe
	w.Close()
	os.Stdout = originalStdout
	log.SetOutput(originalLogOutput)

	// Obtener la salida capturada
	output := <-outputChan
	r.Close()

	return output
}

var reportDirectories = []string{"./reportes"}

func executeCommands(commandText string) string {
	reportDir := extractReportPath(commandText)
	if reportDir != "" {
		fmt.Printf("Los reportes se guardarán en: %s\n", reportDir)
	}

	lines := strings.Split(commandText, "\n")
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trim), "rep ") {
			parts := utilitiesP2.ParseCommand(trim)
			if len(parts) > 1 {
				params := utilitiesP2.ParseParameters(parts[1:])
				for key, value := range params {
					if strings.ToLower(key) == "path" {
						dir := filepath.Dir(value)
						addReportDirectory(dir)
					}
				}
			}
		}
	}

	var errorCount int = 0
	var successCount int = 0
	var output strings.Builder

	// Identificar líneas que son comandos (no comentarios ni vacías) para contar total
	type CommandInfo struct {
		LineNumber int
		Command    string
	}
	cmdsByLine := make(map[int]CommandInfo)
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		// registrar comando por número de línea
		cmdsByLine[i] = CommandInfo{
			LineNumber: i + 1,
			Command:    trim,
		}
	}

	totalCommands := len(cmdsByLine)

	// Si hay múltiples comandos, escribir encabezado antes de la secuencia
	if totalCommands > 1 {
		output.WriteString(fmt.Sprintf("=== Ejecutando %d comandos ===\n\n", totalCommands))
	}

	// Ejecutar en el orden original de las líneas, preservando comentarios en su lugar
	cmdIndex := 0
	for i, line := range lines {
		trim := strings.TrimSpace(line)

		// Línea vacía -> conservar
		if trim == "" {
			output.WriteString("\n")
			continue
		}

		// Comentario -> mostrar en su posición (no ejecutar)
		if strings.HasPrefix(trim, "#") {
			output.WriteString(fmt.Sprintf("%s\n", trim))
			continue
		}

		// Si es comando reconocido en el mapa (debe serlo)
		if cmdInfo, ok := cmdsByLine[i]; ok {
			cmdIndex++
			// Mostrar progreso con número de línea
			if totalCommands > 1 {
				output.WriteString(fmt.Sprintf("[Comando %d/%d - Línea %d] %s\n",
					cmdIndex, totalCommands, cmdInfo.LineNumber, cmdInfo.Command))
			} else {
				output.WriteString(fmt.Sprintf("[Línea %d] %s\n", cmdInfo.LineNumber, cmdInfo.Command))
			}

			// Capturar la salida de la ejecución del comando
			commandOutput := captureOutput(func() {
				executeCommand(cmdInfo.Command)
			})

			// Verificar si hubo error pero continuar ejecutando
			if strings.Contains(strings.ToLower(commandOutput), "error") {
				errorCount++
				output.WriteString(commandOutput)
				output.WriteString(fmt.Sprintf("⚠️ ERROR en línea %d - Continuando con el siguiente comando...\n", cmdInfo.LineNumber))
			} else {
				successCount++
				if commandOutput != "" {
					output.WriteString(commandOutput)
				}
			}

			// Separador entre comandos si hay múltiples y no es el último
			if totalCommands > 1 && cmdIndex < totalCommands {
				output.WriteString("\n---\n\n")
			} else {
				output.WriteString("\n")
			}
		} else {
			// No se registró como comando (por seguridad), mostrar como texto
			output.WriteString(fmt.Sprintf("%s\n", line))
		}
	}

	// Mensaje final con estadísticas
	if totalCommands > 1 {
		output.WriteString("\n=== RESUMEN DE EJECUCIÓN ===\n")
		output.WriteString(fmt.Sprintf("✅ Comandos exitosos: %d\n", successCount))
		output.WriteString(fmt.Sprintf("❌ Comandos con error: %d\n", errorCount))
		output.WriteString(fmt.Sprintf("📊 Total procesados: %d\n", successCount+errorCount))

		if errorCount > 0 {
			output.WriteString("🔄 Ejecución completada con algunos errores\n")
		} else {
			output.WriteString("🎉 Todos los comandos ejecutados exitosamente\n")
		}
	}

	return output.String()
}

func executeCommand(input string) {
	// Parsear el comando
	cmdParts := utilitiesP2.ParseCommand(input)
	if len(cmdParts) == 0 {
		fmt.Println("Error: Comando vacío o no reconocido")
		return
	}

	// Parsear parámetros en mapa para validar
	params := utilitiesP2.ParseParameters(cmdParts[1:])

	// Definir parámetros permitidos por comando
	allowedParams := map[string][]string{
		"mkdisk":     {"size", "unit", "path", "fit"},
		"rmdisk":     {"path"},
		"fdisk":      {"type", "unit", "name", "size", "path", "fit", "delete", "add"},
		"mount":      {"path", "name"},
		"unmount":    {"id"},
		"mounted":    {},
		"mkfs":       {"id", "type", "fs"},
		"login":      {"user", "pass", "id"},
		"move":       {"path", "destino"},
		"find":       {"path", "name"},
		"chown":      {"path", "usuario", "r"},
		"chmod":      {"path", "ugo", "r"},
		"logout":     {},
		"cat":        {"file1", "file"},
		"edit":       {"path", "contenido"},
		"rename":     {"path", "name"},
		"copy":       {"path", "destino"},
		"mkgrp":      {"name"},
		"rmgrp":      {"name"},
		"mkusr":      {"user", "pass", "grp"},
		"rmusr":      {"user"},
		"chgrp":      {"user", "grp"},
		"mkdir":      {"path", "p"},
		"mkfile":     {"path", "size", "cont", "r"},
		"remove":     {"path"},
		"rep":        {"id", "path", "name", "path_file_ls"},
		"recovery":   {"id"},
		"loss":       {"id"},
		"journaling": {"id"},
	}

	cmdName := strings.ToLower(cmdParts[0])
	if allowed, ok := allowedParams[cmdName]; ok {
		if err := utilitiesP2.ValidateAllowedParams(params, allowed); err != nil {
			// Mostrar error y no ejecutar el comando
			fmt.Printf("Error en parámetros: %v\n", err)
			return
		}
	} else {
		// Si el comando no está en la lista de allowedParams, considerarlo desconocido
		fmt.Printf("Error: Comando '%s' no reconocido\n", cmdParts[0])
		return
	}

	// Ejecutar comando
	switch strings.ToLower(cmdParts[0]) {
	case "mkdisk":
		commandsP2.MKDISK(cmdParts[1:])
	case "rmdisk":
		commandsP2.RMDISK(cmdParts[1:])
	case "fdisk":
		commandsP2.FDISK(cmdParts[1:])
	case "mount":
		commandsP2.MOUNT(cmdParts[1:])
	case "unmount":
		commandsP2.UNMOUNT(cmdParts[1:])
	case "mounted":
		commandsP2.MOUNTED(cmdParts[1:])
	case "mkfs":
		commandsP2.MKFS(cmdParts[1:])
	case "login":
		commandsP2.LOGIN(cmdParts[1:])
	case "logout":
		commandsP2.LOGOUT(cmdParts[1:])
	case "cat":
		commandsP2.CAT(cmdParts[1:])
	case "mkgrp":
		commandsP2.MKGRP(cmdParts[1:])
	case "rmgrp":
		commandsP2.RMGRP(cmdParts[1:])
	case "mkusr":
		commandsP2.MKUSR(cmdParts[1:])
	case "rmusr":
		commandsP2.RMUSR(cmdParts[1:])
	case "chgrp":
		commandsP2.CHGRP(cmdParts[1:])
	case "mkdir":
		commandsP2.MKDIR(cmdParts[1:])
	case "mkfile":
		commandsP2.MKFILE(cmdParts[1:])
	case "rep":
		commandsP2.REP(cmdParts[1:])
	case "edit":
		commandsP2.EDIT(cmdParts[1:])
	case "rename":
		commandsP2.RENAME(cmdParts[1:])
	case "copy":
		commandsP2.COPY(cmdParts[1:])
	case "remove":
		commandsP2.REMOVE(cmdParts[1:])
	case "move":
		commandsP2.MOVE(cmdParts[1:])
	case "find":
		commandsP2.FIND(cmdParts[1:])
	case "chown":
		commandsP2.CHOWN(cmdParts[1:])
	case "chmod":
		commandsP2.CHMOD(cmdParts[1:])
	case "recovery":
		commandsP2.RECOVERY(cmdParts[1:])
	case "loss":
		commandsP2.LOSS(cmdParts[1:])
	case "journaling":
		commandsP2.JOURNALING(cmdParts[1:])
	default:
		fmt.Printf("Error: Comando '%s' no reconocido\n", cmdParts[0])
	}
}

// enableCORS habilita CORS para permitir peticiones del frontend
func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// executeHandler maneja las peticiones POST para ejecutar comandos
func executeHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)

	// Manejar preflight OPTIONS request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	// Leer el cuerpo de la petición
	var reqBody RequestBody
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&reqBody); err != nil {
		http.Error(w, "Error al decodificar JSON", http.StatusBadRequest)
		return
	}

	// Ejecutar los comandos
	output := executeCommands(reqBody.Commands)

	// Preparar la respuesta
	response := ResponseBody{
		Output: output,
	}

	// Enviar la respuesta
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// healthHandler endpoint simple para verificar que el servidor esté funcionando
func healthHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)

	response := map[string]string{
		"status":    "OK",
		"message":   "Servidor GoDisk funcionando correctamente",
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func startServer() {
	fmt.Println("=== GoDisk - Servidor HTTP ===")
	fmt.Println("Iniciando servidor en http://localhost:8080")

	clearReportDirectories()

	http.HandleFunc("/execute", executeHandler)
	http.HandleFunc("/health", healthHandler)

	http.HandleFunc("/reports/", func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w)

		filename := strings.TrimPrefix(r.URL.Path, "/reports/")
		if filename == "" {
			listReportsHandler(w, r)
			return
		}

		// Buscar el archivo en múltiples directorios
		var foundPath string
		for _, dir := range reportDirectories {
			candidate := filepath.Join(dir, filename)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				foundPath = candidate
				break
			}
		}

		if foundPath == "" {
			http.Error(w, "Archivo no encontrado", http.StatusNotFound)
			return
		}

		// Determinar Content-Type
		contentType := "application/octet-stream"
		switch filepath.Ext(foundPath) {
		case ".png":
			contentType = "image/png"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".gif":
			contentType = "image/gif"
		case ".svg":
			contentType = "image/svg+xml"
		case ".pdf":
			contentType = "application/pdf"
		case ".dot":
			contentType = "text/plain; charset=utf-8"
		case ".txt":
			contentType = "text/plain; charset=utf-8"
			w.Header().Set("Content-Disposition", "inline")
		}

		w.Header().Set("Content-Type", contentType)
		http.ServeFile(w, r, foundPath)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w)
		fmt.Fprintf(w, "GoDisk Server - API Endpoints:\n")
		fmt.Fprintf(w, "POST /execute - Ejecutar comandos\n")
		fmt.Fprintf(w, "GET /health - Estado del servidor\n")
	})
	http.HandleFunc("/filesystem/disks", commandsP2.HandleDisksAPI)
	http.HandleFunc("/filesystem/content", commandsP2.HandleFileSystemContentAPI)
	http.HandleFunc("/filesystem/file-content", commandsP2.HandleFileContentAPI)

	http.HandleFunc("/debug/reports", func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w)

		debugInfo := map[string]interface{}{
			"reportDirectories": reportDirectories,
			"currentWorkingDir": func() string {
				if cwd, err := os.Getwd(); err == nil {
					return cwd
				}
				return "unknown"
			}(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(debugInfo)
	})

	// En main.go, agregar la ruta para journaling
	http.HandleFunc("/journaling", commandsP2.HandleJournalingAPI)

	// Iniciar servidor
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// listReportsHandler lista los reportes disponibles
func listReportsHandler(w http.ResponseWriter, r *http.Request) {

	files, err := os.ReadDir("./reportes")
	if err != nil {
		response := map[string]interface{}{
			"error": "Error al leer directorio de reportes",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}
	// Agrupar archivos por tipos de reporte
	reports := make(map[string][]map[string]string)

	for _, dir := range reportDirectories {
		if err := scanReportsInDirectory(dir, reports); err != nil {
			log.Printf("Error escaneando directorio %s: %v", dir, err)
		}
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		ext := filepath.Ext(name)

		// Ignorar archivos dot (solo nos interesan las imágenes)
		if ext == ".dot" {
			continue
		}

		// Determinar el tipo de reporte por prefijo (mbr_, disk_, etc.)
		var reportType string
		if strings.HasPrefix(name, "mbr_") {
			reportType = "mbr"
		} else if strings.HasPrefix(name, "disk_") {
			reportType = "disk"
		} else if strings.HasPrefix(name, "sb_") {
			reportType = "sb"
		} else if strings.HasPrefix(name, "inode_") {
			reportType = "inode"
		} else if strings.HasPrefix(name, "block_") {
			reportType = "block"
		} else if strings.HasPrefix(name, "bm_block_") {
			reportType = "bm_block"
		} else if strings.HasPrefix(name, "bm_inode_") {
			reportType = "bm_inode"
		} else if strings.HasPrefix(name, "tree_") {
			reportType = "tree"
		} else if strings.HasPrefix(name, "file_") {
			reportType = "file"
		} else if strings.HasPrefix(name, "ls_") {
			reportType = "ls"
		} else {
			reportType = "otros"
		}

		// Obtener info del archivo
		info, err := file.Info()
		if err != nil {
			continue
		}

		fileInfo := map[string]string{
			"name":       name,
			"url":        "/reports/" + name,
			"size":       fmt.Sprintf("%d bytes", info.Size()),
			"created":    info.ModTime().Format("2006-01-02 15:04:05"),
			"extension":  ext,
			"reportType": reportType,
		}

		reports[reportType] = append(reports[reportType], fileInfo)
	}

	for _, dir := range reportDirectories {
		if err := scanReportsInDirectory(dir, reports); err != nil {
			log.Printf("Error escaneando directorio %s: %v", dir, err)
		}
	}

	response := map[string]interface{}{
		"reports": reports,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func extractReportPath(commandText string) string {
	lines := strings.Split(commandText, "\n")

	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}

		// Buscar comandos REP
		if strings.HasPrefix(strings.ToLower(trim), "rep ") {
			// Parsear parámetros del comando REP
			parts := utilitiesP2.ParseCommand(trim)
			if len(parts) > 1 {
				params := utilitiesP2.ParseParameters(parts[1:])

				// Buscar parámetro path (case insensitive)
				for key, value := range params {
					if strings.ToLower(key) == "path" {
						return filepath.Dir(value) // Retornar el directorio
					}
				}
			}
		}
	}

	return "./reportes" // Fallback a la carpeta por defecto
}

var seenFilesGlobal = make(map[string]bool)

func scanReportsInDirectory(dirPath string, reports map[string][]map[string]string) error {
	// Verificar que el directorio existe
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		log.Printf("Directorio %s no existe, saltando...", dirPath)
		return nil // No es error, simplemente no existe
	}

	files, err := os.ReadDir(dirPath)
	if err != nil {
		return err
	}

	// Crear un mapa para evitar duplicados
	seenFiles := make(map[string]bool)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		ext := strings.ToLower(filepath.Ext(name))

		// Solo procesar archivos de imagen
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".svg" && ext != ".txt" {
			continue
		}

		// Evitar agregar archivos duplicados
		if seenFiles[name] {
			continue
		}
		seenFiles[name] = true

		if seenFilesGlobal[name] {
			continue
		}
		seenFilesGlobal[name] = true

		// Determinar tipo de reporte de manera más flexible
		nameLower := strings.ToLower(name)
		var reportType string

		if strings.Contains(nameLower, "mbr") {
			reportType = "mbr"
		} else if strings.Contains(nameLower, "disk") {
			reportType = "disk"
		} else if strings.Contains(nameLower, "bm_inode") || strings.Contains(nameLower, "bitmap_inode") {
			reportType = "bm_inode"
		} else if strings.Contains(nameLower, "bm_block") || strings.Contains(nameLower, "bitmap_block") {
			reportType = "bm_block"
		} else if strings.Contains(nameLower, "inode") {
			reportType = "inode"
		} else if strings.Contains(nameLower, "block") {
			reportType = "block"
		} else if strings.Contains(nameLower, "tree") {
			reportType = "tree"
		} else if strings.Contains(nameLower, "sb") || strings.Contains(nameLower, "superblock") {
			reportType = "sb"
		} else if strings.Contains(nameLower, "file") {
			reportType = "file"
		} else if strings.Contains(nameLower, "ls") {
			reportType = "ls"
		} else {
			reportType = "otros"
		}

		info, err := file.Info()
		if err != nil {
			log.Printf("Error obteniendo info de %s: %v", name, err)
			continue
		}

		fileInfo := map[string]string{
			"name":       name,
			"url":        "/reports/" + name,
			"fullPath":   filepath.Join(dirPath, name),
			"size":       fmt.Sprintf("%d bytes", info.Size()),
			"created":    info.ModTime().Format("2006-01-02 15:04:05"),
			"extension":  ext,
			"directory":  dirPath,
			"reportType": reportType,
		}

		reports[reportType] = append(reports[reportType], fileInfo)
	}

	return nil
}

func clearReportDirectories() {
	for _, dir := range reportDirectories {
		files, err := os.ReadDir(dir)
		if err != nil {
			// Si el directorio no existe, continuar con el siguiente
			if os.IsNotExist(err) {
				continue
			}
			log.Printf("Error al leer el directorio %s: %v", dir, err)
			continue
		}

		for _, file := range files {
			if !file.IsDir() {
				filePath := filepath.Join(dir, file.Name())
				err := os.Remove(filePath)
				if err != nil {
					log.Printf("Error al eliminar el archivo %s: %v", filePath, err)
				} else {
					log.Printf("Archivo eliminado: %s", filePath)
				}
			}
		}
		log.Printf("Directorio limpiado: %s", dir)
	}
}

func addReportDirectory(dir string) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return
	}

	// Verificar si ya existe
	for _, existing := range reportDirectories {
		if existing == absDir {
			return
		}
	}

	// Verificar que el directorio existe
	if _, err := os.Stat(absDir); err == nil {
		reportDirectories = append(reportDirectories, absDir)
		log.Printf("Directorio de reportes agregado: %s", absDir)
	}
}
