import React, { useState, useEffect } from 'react';
import './FileSystemExplorer.css';

const FileSystemExplorer = ({ session }) => {
  const [step, setStep] = useState(1); // 1: discos, 2: particiones, 3: navegación
  const [availableDisks, setAvailableDisks] = useState([]);
  const [selectedDisk, setSelectedDisk] = useState(null);
  const [selectedPartition, setSelectedPartition] = useState(null);
  const [currentPath, setCurrentPath] = useState('/');
  const [directoryContent, setDirectoryContent] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  
  // Estados para el modal de archivos
  const [viewingFile, setViewingFile] = useState(null);
  const [fileContentLoading, setFileContentLoading] = useState(false);

  // Cargar discos al montar el componente
  useEffect(() => {
    loadAvailableDisks();
  }, []);

  const loadAvailableDisks = async () => {
    try {
      setLoading(true);
      setError('');
      
      const resp = await fetch('http://localhost:8080/filesystem/disks');
      if (!resp.ok) {
        throw new Error(`Error ${resp.status}: ${resp.statusText}`);
      }

      const data = await resp.json();
      if (data.success) {
        setAvailableDisks(data.disks || []);
      } else {
        setError('Error cargando información de discos');
      }
    } catch (err) {
      setError(`Error de conexión: ${err.message}`);
      console.error('Error cargando discos:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleDiskSelect = (disk) => {
    setSelectedDisk(disk);
    setSelectedPartition(null);
    setStep(2);
    setError('');
  };

  const handlePartitionSelect = (partition) => {
    if (!partition.is_mounted) {
      setError('La partición debe estar montada para ser explorada');
      return;
    }

    if (session.id !== partition.mounted_id) {
      setError('Debe iniciar sesión en esta partición para explorarla');
      return;
    }

    setSelectedPartition(partition);
    setCurrentPath('/');
    setStep(3);
    setError('');
    loadDirectoryContent('/', partition.mounted_id);
  };

  const loadDirectoryContent = async (path, partitionID = null) => {
    try {
      setLoading(true);
      setError('');

      const pid = partitionID || selectedPartition?.mounted_id;
      if (!pid) {
        throw new Error('No hay partición seleccionada');
      }

      const url = `http://localhost:8080/filesystem/content?partition_id=${pid}&path=${encodeURIComponent(path)}`;
      const resp = await fetch(url);
      
      if (!resp.ok) {
        throw new Error(`Error ${resp.status}: ${resp.statusText}`);
      }

      const data = await resp.json();
      if (data.success) {
        setDirectoryContent(data.items || []);
        setCurrentPath(path);
      } else {
        setError(data.error || 'Error cargando contenido del directorio');
      }
    } catch (err) {
      setError(`Error cargando directorio: ${err.message}`);
      console.error('Error cargando directorio:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleItemClick = (item) => {
    if (item.is_directory) {
      loadDirectoryContent(item.path);
    } else {
      // Mostrar contenido del archivo
      loadFileContent(item);
    }
  };

  const loadFileContent = async (file) => {
    try {
      setFileContentLoading(true);
      setError('');
      
      // Usar el nuevo endpoint que utiliza CAT
      const url = `http://localhost:8080/filesystem/file-content?partition_id=${selectedPartition.mounted_id}&file_path=${encodeURIComponent(file.path)}`;
      const resp = await fetch(url);
      
      if (!resp.ok) {
        throw new Error(`Error ${resp.status}: ${resp.statusText}`);
      }

      const data = await resp.json();
      if (data.success) {
        setViewingFile({
          ...file,
          content: data.content,
          fileInfo: data.fileInfo
        });
      } else {
        setError(data.error || 'Error cargando contenido del archivo');
      }
    } catch (err) {
      setError(`Error cargando archivo: ${err.message}`);
      console.error('Error cargando archivo:', err);
    } finally {
      setFileContentLoading(false);
    }
  };

  const handleBackToParent = () => {
    if (currentPath !== '/') {
      const parts = currentPath.split('/').filter(p => p);
      const parentPath = parts.length <= 1 ? '/' : '/' + parts.slice(0, -1).join('/');
      loadDirectoryContent(parentPath);
    }
  };

  const handleBack = () => {
    if (step === 3) {
      setStep(2);
      setCurrentPath('/');
      setDirectoryContent([]);
      setViewingFile(null); // Cambié de setFileContent a setViewingFile
    } else if (step === 2) {
      setStep(1);
      setSelectedDisk(null);
      setSelectedPartition(null);
    }
  };

  if (loading && step === 1) {
    return (
      <div className="explorer-container">
        <div className="loading-state">
          <div className="spinner"></div>
          <p>Cargando sistema de archivos...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="explorer-container">
      <div className="explorer-header">
        <h2>📁 Visualizador del Sistema de Archivos</h2>
        <div className="session-indicator">
          <span className="session-badge">
            👤 {session.user} | 🏷️ {session.id}
          </span>
        </div>
      </div>

      {error && (
        <div className="error-message">
          <span>⚠️ {error}</span>
          <button onClick={() => setError('')} className="error-close">✕</button>
        </div>
      )}

      <div className="explorer-content">
        {step === 1 && (
          <DiskSelector 
            disks={availableDisks}
            onDiskSelect={handleDiskSelect}
            onRefresh={loadAvailableDisks}
            loading={loading}
          />
        )}

        {step === 2 && selectedDisk && (
          <PartitionSelector 
            disk={selectedDisk}
            onPartitionSelect={handlePartitionSelect}
            onBack={handleBack}
            currentSession={session}
          />
        )}

        {step === 3 && selectedPartition && (
          <FileSystemNavigator 
            partition={selectedPartition}
            currentPath={currentPath}
            directoryContent={directoryContent}
            onItemClick={handleItemClick}
            onBack={handleBack}
            onBackToParent={handleBackToParent}
            onRefresh={() => loadDirectoryContent(currentPath)}
            loading={loading}
            viewingFile={viewingFile} // Cambié de fileContent a viewingFile
            onCloseFile={() => setViewingFile(null)} // Cambié de setFileContent a setViewingFile
          />
        )}
      </div>

      {/* Modal de contenido de archivo - movido aquí para estar disponible globalmente */}
      {viewingFile && (
        <FileContentModal 
          file={viewingFile}
          onClose={() => setViewingFile(null)}
          loading={fileContentLoading}
        />
      )}
    </div>
  );
};

// Componente 1: Selector de Discos
const DiskSelector = ({ disks, onDiskSelect, onRefresh, loading }) => {
  return (
    <div className="disk-selector">
      <div className="selector-header">
        <h3>1. Selección de Disco</h3>
        <p>Seleccione el disco que desea explorar</p>
        <button onClick={onRefresh} className="refresh-btn" disabled={loading}>
          🔄 Actualizar
        </button>
      </div>

      <div className="disks-grid">
        {disks.length === 0 ? (
          <div className="no-disks">
            <div className="no-disks-icon">💾</div>
            <p>No se encontraron discos disponibles</p>
            <small>Cree discos usando el comando mkdisk</small>
          </div>
        ) : (
          disks.map((disk, index) => (
            <DiskCard 
              key={index}
              disk={disk}
              onClick={() => onDiskSelect(disk)}
            />
          ))
        )}
      </div>

      <div className="selector-info">
        <div className="info-card">
          <h4>ℹ️ Información</h4>
          <ul>
            <li>Los discos muestran información básica como capacidad y fit</li>
            <li>Se indica el número de particiones montadas</li>
            <li>Solo se pueden explorar particiones montadas y formateadas</li>
          </ul>
        </div>
      </div>
    </div>
  );
};

// Componente para mostrar tarjeta de disco
const DiskCard = ({ disk, onClick }) => {
  const formatSize = (bytes) => {
    if (bytes >= 1024 * 1024 * 1024) {
      return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
    } else if (bytes >= 1024 * 1024) {
      return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    } else if (bytes >= 1024) {
      return `${(bytes / 1024).toFixed(1)} KB`;
    }
    return `${bytes} B`;
  };

  const getFitName = (fit) => {
    const fitNames = {
      'F': 'First Fit',
      'B': 'Best Fit', 
      'W': 'Worst Fit'
    };
    return fitNames[fit] || fit;
  };

  return (
    <div className="disk-card" onClick={onClick}>
      <div className="disk-icon">
        <div className="disk-image">💾</div>
      </div>
      <div className="disk-info">
        <h4>{disk.name}</h4>
        <p className="disk-path">{disk.path}</p>
        
        <div className="disk-stats">
          <div className="stat">
            <span className="stat-label">Tamaño:</span>
            <span className="stat-value">{formatSize(disk.size)}</span>
          </div>
          <div className="stat">
            <span className="stat-label">Ajuste:</span>
            <span className="stat-value">{getFitName(disk.fit)}</span>
          </div>
          <div className="stat">
            <span className="stat-label">Particiones:</span>
            <span className="stat-value">{disk.partitions.length}</span>
          </div>
          <div className="stat">
            <span className="stat-label">Montadas:</span>
            <span className="stat-value">{disk.mounted_count}</span>
          </div>
        </div>

        {disk.mounted_count > 0 && (
          <div className="mounted-partitions">
            {disk.partitions
              .filter(p => p.is_mounted)
              .slice(0, 3)
              .map((partition, idx) => (
                <span key={idx} className="partition-badge mounted">
                  {partition.mounted_id}
                </span>
              ))}
            {disk.mounted_count > 3 && (
              <span className="partition-badge more">+{disk.mounted_count - 3}</span>
            )}
          </div>
        )}
      </div>
      <div className="disk-arrow">→</div>
    </div>
  );
};

// Componente 2: Selector de Particiones
const PartitionSelector = ({ disk, onPartitionSelect, onBack, currentSession }) => {
  const formatSize = (bytes) => {
    if (bytes >= 1024 * 1024 * 1024) {
      return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
    } else if (bytes >= 1024 * 1024) {
      return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    } else if (bytes >= 1024) {
      return `${(bytes / 1024).toFixed(1)} KB`;
    }
    return `${bytes} B`;
  };

  return (
    <div className="partition-selector">
      <div className="selector-header">
        <h3>2. Selección de Partición</h3>
        <p>Seleccione la partición que desea explorar en el disco <strong>{disk.name}</strong></p>
        <button onClick={onBack} className="back-btn">
          ← Volver a discos
        </button>
      </div>

      <div className="partitions-grid">
        {disk.partitions.length === 0 ? (
          <div className="no-partitions">
            <div className="no-partitions-icon">🗂️</div>
            <p>No se encontraron particiones en este disco</p>
            <small>Cree particiones usando el comando fdisk</small>
          </div>
        ) : (
          disk.partitions.map((partition, index) => (
            <PartitionCard 
              key={index}
              partition={partition}
              onClick={() => onPartitionSelect(partition)}
              currentSession={currentSession}
            />
          ))
        )}
      </div>

      <div className="disk-summary">
        <h4>📊 Resumen del Disco</h4>
        <div className="summary-stats">
          <div className="summary-stat">
            <span>Tamaño Total:</span>
            <span>{formatSize(disk.size)}</span>
          </div>
          <div className="summary-stat">
            <span>Particiones:</span>
            <span>{disk.partitions.length}</span>
          </div>
          <div className="summary-stat">
            <span>Montadas:</span>
            <span>{disk.mounted_count}</span>
          </div>
          <div className="summary-stat">
            <span>Ajuste:</span>
            <span>{disk.fit}</span>
          </div>
        </div>
      </div>
    </div>
  );
};

// Componente para mostrar tarjeta de partición
const PartitionCard = ({ partition, onClick, currentSession }) => {
  const formatSize = (bytes) => {
    if (bytes >= 1024 * 1024 * 1024) {
      return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
    } else if (bytes >= 1024 * 1024) {
      return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    } else if (bytes >= 1024) {
      return `${(bytes / 1024).toFixed(1)} KB`;
    }
    return `${bytes} B`;
  };

  const canExplore = partition.is_mounted && partition.mounted_id === currentSession.id;
  const isCurrentSession = partition.mounted_id === currentSession.id;

  return (
    <div 
      className={`partition-card ${partition.type.toLowerCase()} ${canExplore ? 'explorable' : 'disabled'}`}
      onClick={canExplore ? onClick : undefined}
    >
      <div className="partition-icon">
        <div className="partition-image">
          {partition.type === 'Primaria' && '🗂️'}
          {partition.type === 'Extendida' && '📂'}
          {partition.type === 'Lógica' && '📄'}
        </div>
        {partition.is_mounted && (
          <div className="mounted-indicator">🔗</div>
        )}
      </div>
      
      <div className="partition-info">
        <h4>{partition.name}</h4>
        <div className="partition-stats">
          <div className="stat">
            <span className="stat-label">Tipo:</span>
            <span className="stat-value">{partition.type}</span>
          </div>
          <div className="stat">
            <span className="stat-label">Tamaño:</span>
            <span className="stat-value">{formatSize(partition.size)}</span>
          </div>
          <div className="stat">
            <span className="stat-label">Estado:</span>
            <span className={`stat-value ${partition.is_mounted ? 'mounted' : 'unmounted'}`}>
              {partition.is_mounted ? 'Montada' : 'No montada'}
            </span>
          </div>
          {partition.is_mounted && (
            <div className="stat">
              <span className="stat-label">ID:</span>
              <span className={`stat-value ${isCurrentSession ? 'current-session' : ''}`}>
                {partition.mounted_id}
              </span>
            </div>
          )}
          {partition.file_system && (
            <div className="stat">
              <span className="stat-label">Sistema:</span>
              <span className="stat-value">{partition.file_system}</span>
            </div>
          )}
        </div>

        {!canExplore && (
          <div className="partition-warning">
            {!partition.is_mounted ? 
              '⚠️ Partición no montada' : 
              '⚠️ Inicie sesión en esta partición'
            }
          </div>
        )}
      </div>
      
      {canExplore && (
        <div className="partition-arrow">→</div>
      )}
    </div>
  );
};

// Componente 3: Navegador del Sistema de Archivos
const FileSystemNavigator = ({ 
  partition, 
  currentPath, 
  directoryContent, 
  onItemClick, 
  onBack, 
  onBackToParent,
  onRefresh,
  loading,
  viewingFile, // Cambié de fileContent a viewingFile
  onCloseFile
}) => {
  return (
    <div className="filesystem-navigator">
      <div className="navigator-header">
        <h3>3. Navegación del Sistema de Archivos</h3>
        <div className="navigator-controls">
          <button onClick={onBack} className="back-btn">
            ← Volver a particiones
          </button>
          <button onClick={onRefresh} className="refresh-btn" disabled={loading}>
            🔄 Actualizar
          </button>
        </div>
      </div>

      <div className="partition-info-bar">
        <div className="partition-details">
          <span className="partition-name">📂 {partition.name}</span>
          <span className="partition-type">({partition.type})</span>
          <span className="partition-id">ID: {partition.mounted_id}</span>
        </div>
      </div>

      <div className="navigation-bar">
        <button 
          className="nav-btn"
          onClick={() => onItemClick({ path: '/', is_directory: true })}
          disabled={currentPath === '/'}
        >
          🏠 Raíz
        </button>
        <button 
          className="nav-btn"
          onClick={onBackToParent}
          disabled={currentPath === '/'}
        >
          ↩️ Subir
        </button>
        <div className="path-breadcrumb">
          <span className="breadcrumb-root" onClick={() => onItemClick({ path: '/', is_directory: true })}>
            /
          </span>
          {currentPath !== '/' && currentPath.split('/').filter(p => p).map((part, idx, arr) => (
            <span key={idx} className="breadcrumb-item">
              <span className="breadcrumb-separator">/</span>
              <span 
                className="breadcrumb-link"
                onClick={() => {
                  const path = '/' + arr.slice(0, idx + 1).join('/');
                  onItemClick({ path, is_directory: true });
                }}
              >
                {part}
              </span>
            </span>
          ))}
        </div>
      </div>

      <div className="content-area">
        {loading ? (
          <div className="loading-state">
            <div className="spinner"></div>
            <p>Cargando contenido...</p>
          </div>
        ) : (
          <div className="directory-content">
            <div className="content-header">
              <h4>📁 {currentPath}</h4>
              <span className="item-count">{directoryContent.length} elementos</span>
            </div>
            
            <div className="items-grid">
              {directoryContent.length === 0 ? (
                <div className="empty-directory">
                  <p>📭 Directorio vacío</p>
                </div>
              ) : (
                directoryContent.map((item, index) => (
                  <FileSystemItem 
                    key={index}
                    item={item}
                    onClick={() => onItemClick(item)}
                  />
                ))
              )}
            </div>
          </div>
        )}
      </div>

      {/* Eliminé el modal de aquí ya que ahora está en el componente principal */}
    </div>
  );
};

// Componente para mostrar elementos del sistema de archivos
const FileSystemItem = ({ item, onClick }) => {
  const formatSize = (bytes) => {
    if (bytes >= 1024 * 1024) {
      return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    } else if (bytes >= 1024) {
      return `${(bytes / 1024).toFixed(1)} KB`;
    }
    return `${bytes} B`;
  };

  return (
    <div 
      className={`filesystem-item ${item.is_directory ? 'directory' : 'file'}`}
      onClick={onClick}
    >
      <div className="item-icon">
        {item.is_directory ? '📁' : '📄'}
      </div>
      <div className="item-info">
        <div className="item-name">{item.name}</div>
        <div className="item-details">
          <span className="item-size">
            {item.is_directory ? 'Directorio' : formatSize(item.size)}
          </span>
          <span className="item-permissions">{item.permissions}</span>
        </div>
        <div className="item-owner">
          👤 {item.owner}:{item.group}
        </div>
      </div>
      {item.is_directory && (
        <div className="item-arrow">→</div>
      )}
    </div>
  );
};

// Componente para mostrar contenido de archivos - MEJORADO
const FileContentModal = ({ file, onClose, loading }) => {
  const formatSize = (bytes) => {
    if (bytes >= 1024 * 1024) {
      return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
    } else if (bytes >= 1024) {
      return `${(bytes / 1024).toFixed(2)} KB`;
    }
    return `${bytes} B`;
  };

  const isTextFile = (filename) => {
    const textExtensions = ['.txt', '.md', '.json', '.xml', '.html', '.css', '.js', '.py', '.java', '.c', '.cpp', '.h'];
    const ext = filename.toLowerCase().substring(filename.lastIndexOf('.'));
    return textExtensions.includes(ext) || !filename.includes('.');
  };

  const isBinaryFile = (content) => {
    // Detectar si el contenido es binario
    return /[\x00-\x08\x0E-\x1F\x7F]/.test(content);
  };

  if (loading) {
    return (
      <div className="file-content-modal">
        <div className="modal-backdrop" onClick={onClose}></div>
        <div className="modal-content">
          <div className="modal-header">
            <h4>📄 Cargando...</h4>
            <button onClick={onClose} className="close-btn">✕</button>
          </div>
          <div className="modal-body">
            <div className="loading-state">
              <div className="spinner"></div>
              <p>Cargando contenido del archivo...</p>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="file-content-modal">
      <div className="modal-backdrop" onClick={onClose}></div>
      <div className="modal-content">
        <div className="modal-header">
          <h4>📄 {file.name}</h4>
          <button onClick={onClose} className="close-btn">✕</button>
        </div>
        <div className="modal-body">
          <div className="file-info">
            <span>📏 Tamaño: {formatSize(file.size)}</span>
            <span>🔒 Permisos: {file.permissions}</span>
            <span>👤 Propietario: {file.owner}:{file.group}</span>
            <span>📍 Ruta: {file.path}</span>
          </div>
          <div className="file-content">
            {!file.content ? (
              <div className="empty-file">
                <p>📭 Archivo vacío</p>
              </div>
            ) : isBinaryFile(file.content) ? (
              <div className="binary-file">
                <p>🔒 Archivo binario</p>
                <p>Este archivo contiene datos binarios que no se pueden mostrar como texto.</p>
                <div className="file-preview">
                  <h5>Vista hexadecimal (primeros 256 bytes):</h5>
                  <pre className="hex-preview">
                    {file.content.substring(0, 256)
                      .split('')
                      .map((char, idx) => {
                        if (idx % 16 === 0) return '\n';
                        return char.charCodeAt(0).toString(16).padStart(2, '0') + ' ';
                      })
                      .join('')
                    }
                  </pre>
                </div>
              </div>
            ) : (
              <div className="text-file">
                <div className="content-stats">
                  <span>📝 Líneas: {file.content.split('\n').length}</span>
                  <span>🔤 Caracteres: {file.content.length}</span>
                  <span>📊 Tipo: {isTextFile(file.name) ? 'Texto' : 'Desconocido'}</span>
                </div>
                <pre className="file-text-content">{file.content}</pre>
              </div>
            )}
          </div>
          <div className="file-actions">
            <button 
              className="action-btn" 
              onClick={() => navigator.clipboard.writeText(file.content)}
              disabled={!file.content || isBinaryFile(file.content)}
            >
              📋 Copiar contenido
            </button>
            <button 
              className="action-btn secondary" 
              onClick={() => {
                const blob = new Blob([file.content], { type: 'text/plain' });
                const url = URL.createObjectURL(blob);
                const a = document.createElement('a');
                a.href = url;
                a.download = file.name;
                a.click();
                URL.revokeObjectURL(url);
              }}
              disabled={!file.content}
            >
              💾 Descargar
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default FileSystemExplorer;