import React, { useState, useEffect } from 'react';
import './JournalingViewer.css';

const JournalingViewer = ({ session }) => {
  const [journalData, setJournalData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [selectedPartition, setSelectedPartition] = useState('');
  const [mountedPartitions, setMountedPartitions] = useState([]);
  const [sortConfig, setSortConfig] = useState({ key: 'count', direction: 'desc' });
  const [filterText, setFilterText] = useState('');
  const [autoRefresh, setAutoRefresh] = useState(false);

  // Obtener particiones montadas al cargar
  useEffect(() => {
    fetchMountedPartitions();
  }, []);

  // Auto-refresh cada 30 segundos si está habilitado
  useEffect(() => {
    let interval;
    if (autoRefresh && selectedPartition) {
      interval = setInterval(() => {
        fetchJournaling(selectedPartition, true); // silent refresh
      }, 30000);
    }
    return () => {
      if (interval) clearInterval(interval);
    };
  }, [autoRefresh, selectedPartition]);

  const fetchMountedPartitions = async () => {
    try {
      const response = await fetch('http://18.117.250.26:8080/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ commands: 'mounted' }),
      });
      
      if (!response.ok) throw new Error(`Error: ${response.status}`);
      
      const data = await response.json();
      const outputLines = data.output.split('\n');
      const mountedIdsLine = outputLines.find(line => line.startsWith('IDs montados:'));
      
      if (mountedIdsLine) {
        const ids = mountedIdsLine
          .replace('IDs montados:', '')
          .split(',')
          .map(id => id.trim())
          .filter(id => id);
        
        setMountedPartitions(ids);
        
        // Auto-seleccionar la partición de la sesión si está disponible
        if (session && ids.includes(session.id)) {
          setSelectedPartition(session.id);
        }
      }
    } catch (err) {
      console.error('Error obteniendo particiones montadas:', err);
    }
  };

  const getOperationDetails = (entry) => {
    const details = [];
    
    if (entry.content) {
        // Parsear detalles del contenido
        const contentParts = entry.content.split(' ');
        contentParts.forEach(part => {
        if (part.includes('=')) {
            const [key, value] = part.split('=');
            details.push({ key, value });
        }
        });
    }
    
    return details;
    };

  const fetchJournaling = async (partitionId, silent = false) => {
    if (!partitionId) return;
    
    if (!silent) {
      setLoading(true);
      setError(null);
    }
    
    try {
      const response = await fetch(`http://18.117.250.26:8080/journaling?id=${partitionId}`);
      
      if (!response.ok) {
        throw new Error(`Error del servidor: ${response.status}`);
      }
      
      const data = await response.json();
      
      if (data.success) {
        setJournalData(data);
      } else {
        setError(data.message || 'Error desconocido');
        setJournalData(null);
      }
    } catch (err) {
      setError(`Error de conexión: ${err.message}`);
      setJournalData(null);
    } finally {
      if (!silent) setLoading(false);
    }
  };

  const handlePartitionChange = (e) => {
    const partitionId = e.target.value;
    setSelectedPartition(partitionId);
    if (partitionId) {
      fetchJournaling(partitionId);
    } else {
      setJournalData(null);
    }
  };

  const handleSort = (key) => {
    let direction = 'asc';
    if (sortConfig.key === key && sortConfig.direction === 'asc') {
      direction = 'desc';
    }
    setSortConfig({ key, direction });
  };

  const getOperationIcon = (operation) => {
    const icons = {
      'mkdir': '📁',
      'mkfile': '📄',
      'rmdir': '🗑️',
      'rmfile': '🗑️',
      'edit': '✏️',
      'rename': '📝',
      'chmod': '🔒',
      'chown': '👤',
      'mkgrp': '👥',
      'mkusr': '👤',
      'login': '🔐',
      'logout': '🚪',
      'recovery': '🔧',
      'format': '💾',
      'copy': '📋',
      'move': '➡️',
      'mount': '💿',
      'unmount': '⏏️',
      'loss': '💥'
    };
    return icons[operation] || '📋';
  };

  const getOperationColor = (operation) => {
    const colors = {
      'mkdir': '#10b981', // green
      'mkfile': '#3b82f6', // blue
      'rmdir': '#ef4444', // red
      'rmfile': '#ef4444', // red
      'edit': '#f59e0b', // yellow
      'rename': '#8b5cf6', // purple
      'chmod': '#06b6d4', // cyan
      'chown': '#06b6d4', // cyan
      'login': '#10b981', // green
      'logout': '#f59e0b', // yellow
      'recovery': '#ef4444', // red
      'format': '#6b7280', // gray
      'copy': '#3b82f6', // blue
      'move': '#8b5cf6', // purple
      'mount': '#10b981', // green
      'unmount': '#f59e0b' // yellow
    };
    return colors[operation] || '#6b7280';
  };

  const sortedAndFilteredEntries = React.useMemo(() => {
    if (!journalData?.entries) return [];
    
    let filtered = journalData.entries;
    
    // Aplicar filtro de texto
    if (filterText) {
      const searchTerm = filterText.toLowerCase();
      filtered = filtered.filter(entry => 
        entry.operation.toLowerCase().includes(searchTerm) ||
        entry.path.toLowerCase().includes(searchTerm) ||
        entry.content.toLowerCase().includes(searchTerm)
      );
    }
    
    // Aplicar ordenamiento
    const sorted = [...filtered].sort((a, b) => {
      const aValue = a[sortConfig.key];
      const bValue = b[sortConfig.key];
      
      if (typeof aValue === 'string') {
        return sortConfig.direction === 'asc' 
          ? aValue.localeCompare(bValue)
          : bValue.localeCompare(aValue);
      }
      
      return sortConfig.direction === 'asc' 
        ? aValue - bValue 
        : bValue - aValue;
    });
    
    return sorted;
  }, [journalData, sortConfig, filterText]);

  const getSortIcon = (columnKey) => {
    if (sortConfig.key !== columnKey) return '↕️';
    return sortConfig.direction === 'asc' ? '↑' : '↓';
  };

  return (
    <div className="journaling-container">
      <div className="journaling-header">
        <div className="header-content">
          <h2>📜 Journal de Transacciones</h2>
          <p>Visualiza todas las transacciones realizadas en el sistema de archivos EXT3</p>
        </div>
        
        <div className="header-controls">
          <div className="control-group">
            <label>Partición:</label>
            <select 
              value={selectedPartition} 
              onChange={handlePartitionChange}
              className="partition-selector"
            >
              <option value="">-- Seleccionar partición --</option>
              {mountedPartitions.map(id => (
                <option key={id} value={id}>{id}</option>
              ))}
            </select>
          </div>
          
          <div className="control-group">
            <label>
              <input 
                type="checkbox" 
                checked={autoRefresh}
                onChange={(e) => setAutoRefresh(e.target.checked)}
              />
              Auto-actualizar (30s)
            </label>
          </div>
          
          <button 
            onClick={() => fetchJournaling(selectedPartition)}
            disabled={!selectedPartition || loading}
            className="refresh-btn"
          >
            {loading ? '🔄' : '🔃'} Actualizar
          </button>
        </div>
      </div>

      {selectedPartition && (
        <div className="search-controls">
          <div className="search-group">
            <input
              type="text"
              placeholder="Buscar en operación, ruta o contenido..."
              value={filterText}
              onChange={(e) => setFilterText(e.target.value)}
              className="search-input"
            />
            {filterText && (
              <button 
                onClick={() => setFilterText('')}
                className="clear-search"
              >
                ✕
              </button>
            )}
          </div>
        </div>
      )}

      {loading && (
        <div className="loading-container">
          <div className="loading-spinner"></div>
          <p>Cargando transacciones del journal...</p>
        </div>
      )}

      {error && (
        <div className="error-container">
          <div className="error-icon">⚠️</div>
          <div>
            <h3>Error al cargar el journal</h3>
            <p>{error}</p>
          </div>
          <button onClick={() => setError(null)} className="close-error">✕</button>
        </div>
      )}

      {journalData && !loading && (
        <div className="journal-content">
          <div className="journal-stats">
            <div className="stat-item">
              <span className="stat-label">Partición:</span>
              <span className="stat-value">{journalData.partition_id}</span>
            </div>
            <div className="stat-item">
              <span className="stat-label">Sistema:</span>
              <span className="stat-value">{journalData.filesystem}</span>
            </div>
            <div className="stat-item">
              <span className="stat-label">Total de transacciones:</span>
              <span className="stat-value">{journalData.total_count}</span>
            </div>
            <div className="stat-item">
              <span className="stat-label">Mostrando:</span>
              <span className="stat-value">{sortedAndFilteredEntries.length}</span>
            </div>
          </div>

          {sortedAndFilteredEntries.length === 0 ? (
            <div className="no-data">
              <div className="no-data-icon">📋</div>
              <h3>No hay transacciones que mostrar</h3>
              <p>
                {filterText 
                  ? 'No se encontraron transacciones que coincidan con el filtro' 
                  : 'No hay transacciones registradas en el journal'
                }
              </p>
            </div>
          ) : (
            <div className="journal-table-container">
              <table className="journal-table">
                <thead>
                  <tr>
                    <th onClick={() => handleSort('operation')} className="sortable">
                      Operación {getSortIcon('operation')}
                    </th>
                    <th onClick={() => handleSort('path')} className="sortable">
                      Path {getSortIcon('path')}
                    </th>
                    <th onClick={() => handleSort('content')} className="sortable">
                      Contenido {getSortIcon('content')}
                    </th>
                    <th onClick={() => handleSort('date')} className="sortable">
                      Fecha {getSortIcon('date')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {sortedAndFilteredEntries.map((entry, index) => (
                    <tr key={`${entry.count}-${index}`} className="journal-row">
                      <td className="operation-cell">
                        <div className="operation-content">
                          <span 
                            className="operation-icon"
                            style={{ color: getOperationColor(entry.operation) }}
                          >
                            {getOperationIcon(entry.operation)}
                          </span>
                          <span 
                            className="operation-text"
                            style={{ color: getOperationColor(entry.operation) }}
                          >
                            {entry.operation}
                          </span>
                        </div>
                      </td>
                      <td className="path-cell" title={entry.path}>
                        {entry.path || '-'}
                      </td>
                      <td className="content-cell" title={entry.content}>
                        <div className="content-container">
                            <span className="content-text">{entry.content || '-'}</span>
                            {getOperationDetails(entry).length > 0 && (
                            <div className="content-details">
                                {getOperationDetails(entry).map((detail, idx) => (
                                <span key={idx} className="detail-tag">
                                    {detail.key}: {detail.value}
                                </span>
                                ))}
                            </div>
                            )}
                        </div>
                        </td>
                      <td className="date-cell">
                        {entry.date}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {!selectedPartition && !loading && (
        <div className="welcome-message">
          <div className="welcome-icon">📜</div>
          <h3>Selecciona una partición</h3>
          <p>Elige una partición montada para ver su journal de transacciones</p>
        </div>
      )}
    </div>
  );
};

export default JournalingViewer;