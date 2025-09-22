import React, { useRef } from 'react';

const FileUploader = ({ onFileLoad }) => {
  const fileInputRef = useRef(null);

  const handleFileSelect = () => {
    fileInputRef.current?.click();
  };

  const handleFileChange = (event) => {
    const file = event.target.files[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (e) => {
        onFileLoad(e.target.result);
        // Reset input para permitir cargar el mismo archivo de nuevo
        event.target.value = '';
      };
      reader.readAsText(file);
    }
  };

  return (
    <>
      <input
        type="file"
        ref={fileInputRef}
        onChange={handleFileChange}
        accept=".smia"
        style={{ display: 'none' }}
      />
      <button 
        onClick={handleFileSelect}
        className="btn-primary"
        title="Cargar archivo de script"
      >
        📁 Cargar script ( .smia )
      </button>
    </>
  );
};

export default FileUploader;