import React, { useState, useEffect, useCallback } from 'react';
import './FileSystemExplorer.css';

const FileSystemExplorer = ({ session }) => {
  const [availableDisks, setAvailableDisks] = useState([]);
  const [selectedDisk, setSelectedDisk] = useState('');
  const [mountedPartitions, setMountedPartitions] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  // Cargar discos disponibles al montar el componente
  useEffect(() => {
    loadAvailableDisks();
    loadMountedPartitions();
  }, []);

  

  const loadAvailableDisks = async () => {
    try {
      setLoading(true);
      
      // Obtener lista de discos montados para inferir qué discos existen
      const resp = await fetch('http://localhost:8080/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ commands: 'mounted' })
      });

      if (resp.ok) {
        const json = await resp.json();
        
        // Extraer información de discos del output
        const disks = extractDisksFromMountedOutput(json.output);
        setAvailableDisks(disks);
      }
    } catch (err) {
      setError('Error cargando discos disponibles');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const loadMountedPartitions = async () => {
    try {
      const resp = await fetch('http://localhost:8080/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ commands: 'mounted' })
      });

      if (resp.ok) {
        const json = await resp.json();
        const partitions = extractPartitionsFromMountedOutput(json.output);
        setMountedPartitions(partitions);
      }
    } catch (err) {
      console.error('Error cargando particiones montadas:', err);
    }
  };

  const extractDisksFromMountedOutput = (output) => {
    const disks = [];
    const lines = output.split('\n');
    
    for (const line of lines) {
      // Buscar líneas que contengan rutas de archivos .mia
      const diskMatch = line.match(/([A-Za-z]:[\\\/][^|]+\.mia|\/[^|]+\.mia)/);
      if (diskMatch) {
        const diskPath = diskMatch[1];
        const diskName = diskPath.split(/[\\\/]/).pop().replace('.mia', '');
        
        if (!disks.find(d => d.path === diskPath)) {
          disks.push({
            name: diskName,
            path: diskPath,
            fullPath: diskPath
          });
        }
      }
    }
    
    return disks;
  };

  const extractPartitionsFromMountedOutput = (output) => {
    const partitions = [];
    const lines = output.split('\n');
    
    for (const line of lines) {
      // Extraer información de particiones montadas
      const partitionMatch = line.match(/([A-Z0-9]+)\s*\|\s*([^|]+)\s*\|\s*([^|]+)\s*\|\s*([^|]+)/);
      if (partitionMatch) {
        partitions.push({
          id: partitionMatch[1].trim(),
          name: partitionMatch[2].trim(),
          path: partitionMatch[3].trim(),
          type: partitionMatch[4].trim()
        });
      }
    }
    
    return partitions;
  };

  const handleDiskSelect = (disk) => {
    setSelectedDisk(disk);
    setError('');
  };

  if (loading) {
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
        </div>
      )}

      <div className="explorer-content">
        {!selectedDisk ? (
          <DiskSelector 
            disks={availableDisks}
            onDiskSelect={handleDiskSelect}
            mountedPartitions={mountedPartitions}
          />
        ) : (
          <FileSystemViewer 
            disk={selectedDisk}
            session={session}
            onBack={() => setSelectedDisk('')}
            mountedPartitions={mountedPartitions}
          />
        )}
      </div>
    </div>
  );
};

// Componente para seleccionar disco
const DiskSelector = ({ disks, onDiskSelect, mountedPartitions }) => {
  return (
    <div className="disk-selector">
      <div className="selector-header">
        <h3>Seleccione el disco que desea visualizar:</h3>
        <p>Los discos disponibles se muestran a continuación</p>
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
              mountedPartitions={mountedPartitions.filter(p => p.path.includes(disk.name))}
            />
          ))
        )}
      </div>

      <div className="selector-info">
        <div className="info-card">
          <h4>ℹ️ Información</h4>
          <ul>
            <li>Seleccione un disco para explorar su contenido</li>
            <li>Solo se mostrarán los discos que tienen particiones montadas</li>
            <li>El explorador es de solo lectura</li>
          </ul>
        </div>
      </div>
    </div>
  );
};

// Componente para mostrar cada disco
const DiskCard = ({ disk, onClick, mountedPartitions }) => {
  return (
    <div className="disk-card" onClick={onClick}>
      <div className="disk-icon">
        <div className="disk-image">💾</div>
      </div>
      <div className="disk-info">
        <h4>{disk.name}</h4>
        <p className="disk-path">{disk.fullPath}</p>
        <div className="disk-stats">
          <span className="partition-count">
            🏷️ {mountedPartitions.length} particion{mountedPartitions.length !== 1 ? 'es' : ''}
          </span>
        </div>
        {mountedPartitions.length > 0 && (
          <div className="mounted-partitions">
            {mountedPartitions.slice(0, 3).map((partition, idx) => (
              <span key={idx} className="partition-badge">
                {partition.id}
              </span>
            ))}
            {mountedPartitions.length > 3 && (
              <span className="partition-badge more">+{mountedPartitions.length - 3}</span>
            )}
          </div>
        )}
      </div>
      <div className="disk-arrow">→</div>
    </div>
  );
};

// Componente para visualizar el sistema de archivos del disco seleccionado
const FileSystemViewer = ({ disk, session, onBack, mountedPartitions }) => {
  const [currentPath, setCurrentPath] = useState('/');
  const [directoryContent, setDirectoryContent] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    loadDirectoryContent('/');
  }, []);

  const loadDirectoryContent = async (path) => {
    try {
      setLoading(true);
      setError('');

      // Usar comando ls para obtener el contenido del directorio
      const lsCommand = `ls -path="${path}"`;
      const resp = await fetch('http://localhost:8080/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ commands: lsCommand })
      });

      if (resp.ok) {
        const json = await resp.json();
        const content = parseDirectoryContent(json.output);
        setDirectoryContent(content);
        setCurrentPath(path);
      } else {
        setError('Error cargando contenido del directorio');
      }
    } catch (err) {
      setError('Error de conexión');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const parseDirectoryContent = (output) => {
    // Parsear el output del comando ls
    const lines = output.split('\n');
    const content = [];

    for (const line of lines) {
      // Buscar patrones de archivos y directorios
      const match = line.match(/([drwx-]{10})\s+(\w+)\s+(\w+)\s+(\d+)\s+(.+)/);
      if (match) {
        const [, permissions, owner, group, size, name] = match;
        content.push({
          name: name.trim(),
          isDirectory: permissions.startsWith('d'),
          permissions,
          owner,
          group,
          size: parseInt(size),
          fullPath: currentPath === '/' ? `/${name.trim()}` : `${currentPath}/${name.trim()}`
        });
      }
    }

    return content;
  };

  const handleItemClick = (item) => {
    if (item.isDirectory) {
      loadDirectoryContent(item.fullPath);
    }
  };

  const handleBackToParent = () => {
    if (currentPath !== '/') {
      const parentPath = currentPath.split('/').slice(0, -1).join('/') || '/';
      loadDirectoryContent(parentPath);
    }
  };

  return (
    <div className="filesystem-viewer">
      <div className="viewer-header">
        <button className="back-btn" onClick={onBack}>
          ← Volver a selección de discos
        </button>
        <div className="current-disk">
          <span className="disk-name">💾 {disk.name}</span>
          <span className="current-path">📂 {currentPath}</span>
        </div>
      </div>

      <div className="navigation-bar">
        <button 
          className="nav-btn"
          onClick={() => loadDirectoryContent('/')}
          disabled={currentPath === '/'}
        >
          🏠 Raíz
        </button>
        <button 
          className="nav-btn"
          onClick={handleBackToParent}
          disabled={currentPath === '/'}
        >
          ↩️ Subir
        </button>
        <div className="path-breadcrumb">
          {currentPath.split('/').filter(p => p).map((part, idx, arr) => (
            <span key={idx} className="breadcrumb-item">
              <span 
                className="breadcrumb-link"
                onClick={() => {
                  const path = '/' + arr.slice(0, idx + 1).join('/');
                  loadDirectoryContent(path);
                }}
              >
                {part}
              </span>
              {idx < arr.length - 1 && <span className="breadcrumb-separator">/</span>}
            </span>
          ))}
        </div>
      </div>

      <div className="directory-content">
        {loading ? (
          <div className="loading-state">
            <div className="spinner"></div>
            <p>Cargando contenido...</p>
          </div>
        ) : error ? (
          <div className="error-state">
            <p>⚠️ {error}</p>
            <button onClick={() => loadDirectoryContent(currentPath)}>
              🔄 Reintentar
            </button>
          </div>
        ) : (
          <div className="items-grid">
            {directoryContent.length === 0 ? (
              <div className="empty-directory">
                <p>📭 Directorio vacío</p>
              </div>
            ) : (
              directoryContent.map((item, index) => (
                <DirectoryItem 
                  key={index}
                  item={item}
                  onClick={() => handleItemClick(item)}
                />
              ))
            )}
          </div>
        )}
      </div>
    </div>
  );
};

// Componente para mostrar elementos del directorio
const DirectoryItem = ({ item, onClick }) => {
  return (
    <div 
      className={`directory-item ${item.isDirectory ? 'directory' : 'file'}`}
      onClick={onClick}
    >
      <div className="item-icon">
        {item.isDirectory ? '📁' : '📄'}
      </div>
      <div className="item-info">
        <div className="item-name">{item.name}</div>
        <div className="item-details">
          <span className="item-size">
            {item.isDirectory ? 'Directorio' : `${item.size} bytes`}
          </span>
          <span className="item-permissions">{item.permissions}</span>
        </div>
        <div className="item-owner">
          👤 {item.owner}:{item.group}
        </div>
      </div>
      {item.isDirectory && (
        <div className="item-arrow">→</div>
      )}
    </div>
  );
};

export default FileSystemExplorer;