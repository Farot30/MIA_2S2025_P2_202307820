import React from 'react';

const CommandInput = ({ value, onChange, placeholder }) => {
  const handleChange = (e) => {
    onChange(e.target.value);
  };

  // Contar comandos válidos (líneas no vacías y que no sean comentarios)
  const countCommands = (text) => {
    return text
      .split('\n')
      .filter(line => line.trim() !== '' && !line.trim().startsWith('#'))
      .length;
  };

  const commandCount = countCommands(value);

  return (
    <div className="command-input-container">
      <textarea
        className="command-textarea"
        value={value}
        onChange={handleChange}
        placeholder={placeholder}
        rows={15}
        spellCheck={false}
      />
      <div className="input-info">
        <span className="line-count">
          Líneas: {value.split('\n').length}
        </span>
        <span className="command-count">
          Comandos: {commandCount}
        </span>
        <span className="char-count">
          Caracteres: {value.length}
        </span>
      </div>
      <div className="input-hint">
        💡 Escribe un comando por línea. Cada línea se ejecutará por separado en orden secuencial.
      </div>
    </div>
  );
};

export default CommandInput;