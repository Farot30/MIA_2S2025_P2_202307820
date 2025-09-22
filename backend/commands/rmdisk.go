package commands

import (
	"MIA_2S2025_P1_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func RMDISK(arg []string) {
	//parsear parametros
	params := utilities.ParseParameters(arg)

	//validar parametros requeridos
	required := []string{"path"}
	if err := utilities.ValidateRequiredParams(params, required); err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	//Obtener los parametros
	path := params["path"]

	if filepath.Ext(path) == ".mia" {
		path = strings.TrimSuffix(path, filepath.Ext(path)) + ".mia"
	}

	removerDisk(path)
}

func removerDisk(path string) {
	// Validar si el disco existe
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("Error: El disco '%s' no existe\n", path)
		return
	}

	// Eliminar el archivo del disco
	err := os.Remove(path)
	if err != nil {
		fmt.Printf("Error al eliminar el disco: %s\n", err)
		return
	}

	fmt.Printf("Disco '%s' eliminado exitosamente\n", path)
}
