package commandsP2

import (
	session "MIA_2S2025_P2_202307820/Session"
	"fmt"
	"strings"
)

// LOGOUT implementa el comando para cerrar sesión
func LOGOUT(args []string) {
	// Este comando no acepta parámetros, pero verificamos si se enviaron
	if len(args) > 0 {
		fmt.Println("Advertencia: El comando 'logout' no acepta parámetros")
		fmt.Println("Los parámetros proporcionados serán ignorados")
	}

	// Verificar que hay una sesión activa
	if !session.IsSessionActive() {
		fmt.Println("Error: No hay ninguna sesión activa")
		fmt.Println("Debe iniciar sesión primero usando: login -user=<usuario> -pass=<contraseña> -id=<partition_id>")
		return
	}

	// Obtener información de la sesión actual antes de cerrarla
	currentSession := session.GetCurrentSession()
	loginTime := session.GetLoginTime()

	// Calcular duración de la sesión
	sessionDuration := session.GetSessionDuration()

	// Cerrar la sesión
	err := session.EndSession()
	if err != nil {
		fmt.Printf("Error cerrando sesión: %s\n", err)
		return
	}

	// Mostrar mensaje de despedida
	fmt.Printf("\n=== SESIÓN CERRADA EXITOSAMENTE ===\n")
	fmt.Printf("Usuario: %s (UID: %d)\n", currentSession.Username, currentSession.UID)
	fmt.Printf("Grupo: %s (GID: %d)\n", currentSession.GroupName, currentSession.GID)
	fmt.Printf("Partición: %s\n", currentSession.PartitionID)
	fmt.Printf("Hora de inicio: %s\n", loginTime)
	fmt.Printf("Duración de sesión: %s\n", sessionDuration)
	fmt.Printf("====================================\n\n")

	fmt.Println("Sesión finalizada. Para volver a usar comandos del sistema, debe iniciar sesión nuevamente.")
}

// ShowSessionSummary muestra un resumen de la sesión que está por cerrarse
func ShowSessionSummary() {
	if !session.IsSessionActive() {
		return
	}

	currentSession := session.GetCurrentSession()
	fmt.Printf("\nResumen de sesión actual:\n")
	fmt.Printf("  Usuario: %s\n", currentSession.Username)
	fmt.Printf("  Grupo: %s\n", currentSession.GroupName)
	fmt.Printf("  Partición: %s\n", currentSession.PartitionID)
	fmt.Printf("  Tiempo activa: %s\n", session.GetSessionDuration())
}

// ForceLogout fuerza el cierre de sesión (para casos de emergencia)
func ForceLogout() error {
	if !session.IsSessionActive() {
		return fmt.Errorf("no hay sesión activa para forzar cierre")
	}

	err := session.ForceEndSession()
	if err != nil {
		return fmt.Errorf("error en cierre forzado: %s", err)
	}

	fmt.Println("Sesión cerrada forzadamente")
	return nil
}

// ConfirmLogout pide confirmación antes de cerrar sesión
func ConfirmLogout() bool {
	if !session.IsSessionActive() {
		return false
	}

	fmt.Print("¿Está seguro que desea cerrar sesión? (s/n): ")

	// Simulamos entrada del usuario (en implementación real sería input)
	response := "s" // Por defecto confirmar

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "s" || response == "si" || response == "y" || response == "yes"
}
