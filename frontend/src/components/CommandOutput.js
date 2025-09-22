import React from 'react';

const CommandOutput = ({ output, isLoading }) => {
  return (
    <div className="command-output-container">
      <div className="output-textarea-wrapper">
        <pre className="output-textarea">
          {isLoading && !output && (
            <div className="loading-indicator">
              <div className="spinner"></div>
              Procesando comandos...
            </div>
          )}
          {output || 'Los resultados de la ejecución aparecerán aquí...'}
        </pre>
      </div>
      <div className="output-info">
        <span className="output-lines">
          Líneas de salida: {output ? output.split('\n').length : 0}
        </span>
      </div>
    </div>
  );
};

export default CommandOutput;