package EstructuraExt3P2

type Information struct {
	I_Operation [10]byte // Tipo de operación (10 caracteres)
	I_Path      [32]byte // Ruta del archivo o carpeta (32 caracteres)
	I_Date      float64  // Fecha y hora de la operación (timestamp float64)
	I_Content   [64]byte // Contenido adicional (64 caracteres)
}

type Journal struct {
	J_Count   int32       // Lleva el conteo del journal que es
	J_Content Information // Estructura de información del journal
}
