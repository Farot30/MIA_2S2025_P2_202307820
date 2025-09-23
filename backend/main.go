package main

import (
	commandsP2 "MIA_2S2025_P2_202307820/commands"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	// Verificar argumentos de línea de comandos
	if len(os.Args) > 1 && os.Args[1] == "server" {
		// Modo servidor HTTP
		startServer()
		return
	}

	// Modo interactivo (consola)
	fmt.Println("=== GoDisk - Sistema de Archivos EXT2 ===")
	fmt.Println("Modo: Consola interactiva")
	fmt.Println("Para iniciar en modo servidor: go run . server")
	fmt.Println()
	fmt.Println("Instrucciones:")
	fmt.Println("- Escriba un comando por línea")
	fmt.Println("- Para ejecutar múltiples comandos, escriba cada uno en una línea separada")
	fmt.Println("- Presione ENTER en una línea vacía para ejecutar todos los comandos")
	fmt.Println("- La ejecución se detendrá automáticamente si hay un error")
	fmt.Println("- Escriba 'exit' o 'quit' para salir")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	lineNumber := 1

	for {
		fmt.Print(">> ")

		// Leer múltiples líneas hasta encontrar una línea vacía
		var inputLines []string
		var lineNumbers []int

		for scanner.Scan() {
			line := scanner.Text()

			// Si la línea está vacía, procesar los comandos acumulados
			if strings.TrimSpace(line) == "" {
				break
			}

			inputLines = append(inputLines, line)
			lineNumbers = append(lineNumbers, lineNumber)
			lineNumber++
		}

		// Si no hay líneas, continuar o salir
		if len(inputLines) == 0 {
			continue
		}

		// Procesar cada línea como un comando separado
		var errorCount int = 0
		var successCount int = 0

		for i, input := range inputLines {
			input = strings.TrimSpace(input)
			if input == "" {
				continue
			}

			// Comandos especiales
			if input == "exit" || input == "quit" {
				fmt.Println("Saliendo del sistema...")
				return
			}

			if input == "server" {
				fmt.Println("Iniciando modo servidor...")
				startServer()
				return
			}

			// Mostrar qué comando se está ejecutando con número de línea
			if len(inputLines) > 1 {
				fmt.Printf("[Comando %d/%d - Línea %d] Ejecutando: %s\n",
					i+1, len(inputLines), lineNumbers[i], input)
			} else {
				fmt.Printf("[Línea %d] Ejecutando: %s\n", lineNumbers[i], input)
			}

			// Parsear el comando
			cmdParts := utilitiesP2.ParseCommand(input)
			if len(cmdParts) == 0 {
				fmt.Printf("❌ Error en línea %d: Comando vacío o inválido\n", lineNumbers[i])
				errorCount++
				// Continuar con el siguiente comando en lugar de break
				if len(inputLines) > 1 && i < len(inputLines)-1 {
					fmt.Println("⏭️ Continuando con el siguiente comando...")
					fmt.Println("---")
				}
				continue
			}

			originalStdout := os.Stdout

			// Crear pipe para capturar output
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Ejecutar comando
			commandHasError := false
			switch strings.ToLower(cmdParts[0]) {
			case "mkdisk":
				commandsP2.MKDISK(cmdParts[1:])
			case "rmdisk":
				commandsP2.RMDISK(cmdParts[1:])
			case "fdisk":
				commandsP2.FDISK(cmdParts[1:])
			case "mount":
				commandsP2.MOUNT(cmdParts[1:])
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
			case "repo":
				commandsP2.REP(cmdParts[1:])
			default:
				fmt.Printf("Error: Comando '%s' no reconocido\n", cmdParts[0])
				commandHasError = true
			}

			// Restaurar stdout y capturar output
			w.Close()
			os.Stdout = originalStdout

			// Leer output capturado
			buf := make([]byte, 4096)
			n, _ := r.Read(buf)
			output := string(buf[:n])
			r.Close()

			// Mostrar output
			fmt.Print(output)

			// Verificar si hubo error pero continuar ejecutando
			if strings.Contains(strings.ToLower(output), "error") || commandHasError {
				fmt.Printf("⚠️ ERROR en línea %d - Continuando con el siguiente comando...\n", lineNumbers[i])
				errorCount++
			} else {
				successCount++
			}

			// Separador visual entre comandos si hay múltiples
			if len(inputLines) > 1 && i < len(inputLines)-1 {
				fmt.Println("---")
			}
		}

		// Mensaje final con estadísticas
		if len(inputLines) > 1 {
			fmt.Printf("\n=== RESUMEN DE EJECUCIÓN ===\n")
			fmt.Printf("✅ Comandos exitosos: %d\n", successCount)
			fmt.Printf("❌ Comandos con error: %d\n", errorCount)
			fmt.Printf("📊 Total procesados: %d\n", successCount+errorCount)

			if errorCount > 0 {
				fmt.Println("🔄 Ejecución completada con algunos errores")
			} else {
				fmt.Println("🎉 Todos los comandos ejecutados exitosamente")
			}
		}
	}
}
