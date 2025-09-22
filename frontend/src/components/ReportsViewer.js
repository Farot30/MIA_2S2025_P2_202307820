import React, { useState, useEffect, useRef, useCallback } from 'react';

const ReportsViewer = () => {
  const [reports, setReports] = useState({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [selectedReport, setSelectedReport] = useState(null);
  const [isGenerating, setIsGenerating] = useState(false);
  const [mountedPartitions, setMountedPartitions] = useState([]);
  const [lastReportCount, setLastReportCount] = useState(0);
  const [newlyGenerated, setNewlyGenerated] = useState(null);
  const [formData, setFormData] = useState({
    reportType: 'mbr',
    partitionId: '',
    filePath: '',
  });

  const intervalRef = useRef(null);

  const fetchReports = useCallback(async (silent = false) => {
      try {
        if (!silent) setLoading(true);
        const response = await fetch('http://localhost:8080/reports/');
        if (!response.ok) {
          throw new Error(`Error: ${response.status}`);
        }
        const data = await response.json();

        console.log('Datos recibidos del backend:', data);

        const newReports = data.reports || {};
        
        // Calcular total de reportes
        const newCount = Object.values(newReports).reduce((total, typeReports) => total + typeReports.length, 0);
        
        // Si hay nuevos reportes, resaltar el más reciente
        if (newCount > lastReportCount && lastReportCount > 0) {
          const allReports = [];
          Object.entries(newReports).forEach(([type, typeReports]) => {
            console.log(`Procesando reportes del tipo: ${type}`, typeReports);
            typeReports.forEach(report => {
              allReports.push({ ...report, type });
            });
          });
          
          // Ordenar por fecha de creación (más reciente primero)
          allReports.sort((a, b) => new Date(b.created) - new Date(a.created));
          
          if (allReports.length > 0) {
            setNewlyGenerated(allReports[0]);
            // Auto-remover el highlight después de 10 segundos
            setTimeout(() => setNewlyGenerated(null), 10000);
          }
        }
        
        setReports(newReports);
        setLastReportCount(newCount);
      } catch (err) {
        if (!silent) {
          setError('Error cargando reportes: ' + err.message);
        }
        console.error(err);
      } finally {
        if (!silent) setLoading(false);
      }
    }, [lastReportCount]);

  // Cargar reportes existentes
  useEffect(() => {
    fetchReports();
    fetchMountedPartitions();

    const intervalId = intervalRef.current

    return () => {
      if (intervalId) {
        clearInterval(intervalId);
      }
    };
  }, [fetchReports]);

  const fetchMountedPartitions = async () => {
    try {
      const response = await fetch('http://localhost:8080/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ commands: 'mounted' }),
      });
      
      if (!response.ok) {
        throw new Error(`Error: ${response.status}`);
      }
      
      const data = await response.json();
      
      // Extraer IDs de particiones montadas del resultado de texto
      const outputLines = data.output.split('\n');
      const mountedIdsLine = outputLines.find(line => line.startsWith('IDs montados:'));
      
      if (mountedIdsLine) {
        const ids = mountedIdsLine
          .replace('IDs montados:', '')
          .split(',')
          .map(id => id.trim())
          .filter(id => id);
        
        setMountedPartitions(ids);
      }
    } catch (err) {
      console.error('Error obteniendo particiones montadas:', err);
    }
  };

  const handleViewReport = (report) => {
    console.log('Reporte seleccionado:', report); 
    setSelectedReport(report);
    // Remover highlight si este reporte estaba resaltado
    if (newlyGenerated && newlyGenerated.name === report.name) {
      setNewlyGenerated(null);
    }
  };

  const handleCloseViewer = () => {
    setSelectedReport(null);
  };

  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setFormData({
      ...formData,
      [name]: value
    });
  };

  const generateReport = async () => {
    const { reportType, partitionId, filePath } = formData;
    
    if (!reportType || !partitionId) {
      alert('Tipo de reporte y ID de partición son obligatorios');
      return;
    }
    
    // Validar si se requiere filePath para ciertos tipos de reporte
    if ((reportType === 'file' || reportType === 'ls') && !filePath) {
      alert('La ruta del archivo/directorio es obligatoria para este tipo de reporte');
      return;
    }
    
    try {
      setIsGenerating(true);
      
      // Generar nombre automático en carpeta reportes
      const timestamp = new Date().toISOString().replace(/[:.]/g, '_');
      let extension = '.png';
      if (['file', 'bm_inode', 'bm_block'].includes(reportType)) {
          extension = '.txt';
      }
      const reportPath = `${reportType}_${partitionId}_${timestamp}${extension}`;
      
      // Construir el comando rep CORRECTAMENTE
      let command = `rep -name=${reportType} -id=${partitionId} -path=${reportPath}`;
      
      // Agregar path_file_ls si es necesario
      if (reportType === 'file' || reportType === 'ls') {
        command += ` -path_file_ls="${filePath}"`;
      }
      
      console.log('Ejecutando comando:', command); // Para debug
      
      const response = await fetch('http://localhost:8080/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ commands: command }),
      });
      
      if (!response.ok) {
        throw new Error(`Error del servidor: ${response.status}`);
      }
      
      const result = await response.json();
      
      if (result.error) {
        throw new Error(result.error);
      }
      
      // Mostrar el output del comando para debug
      console.log('Resultado del comando:', result.output);
      
      // Verificar si hubo errores en el output
      if (result.output && result.output.toLowerCase().includes('error')) {
        throw new Error(`Error generando reporte: ${result.output}`);
      }
      
      // Esperar un poco y recargar reportes
      setTimeout(() => {
        fetchReports();
        setIsGenerating(false);
      }, 1500);
      
    } catch (err) {
      setError('Error generando reporte: ' + err.message);
      setIsGenerating(false);
    }
  };

  const renderReportTypeSelector = () => (
    <select 
      name="reportType" 
      value={formData.reportType} 
      onChange={handleInputChange}
      className="report-selector"
    >
      <option value="mbr">MBR - Master Boot Record</option>
      <option value="disk">DISK - Visualización del Disco</option>
      <option value="inode">INODE - Tabla de Inodos</option>
      <option value="block">BLOCK - Bloques</option>
      <option value="bm_inode">BM_INODE - Bitmap de Inodos (Texto)</option>
      <option value="bm_block">BM_BLOCK - Bitmap de Bloques (Texto)</option>
      <option value="tree">TREE - Árbol de Directorios</option>
      <option value="sb">SB - Superbloque</option>
      <option value="file">FILE - Contenido de Archivo (Texto)</option>
      <option value="ls">LS - Listado de Directorio</option>
    </select>
  );

  const getReportDescription = (type) => {
    const descriptions = {
      mbr: "Estructura del Master Boot Record con particiones y EBRs",
      disk: "Representación gráfica completa del disco y sus particiones",
      inode: "Tabla de inodos del sistema de archivos",
      block: "Información de bloques utilizados (datos, carpetas, indirectos)",
      bm_inode: "Bitmap de inodos (ocupado/libre) - Archivo de texto",
      bm_block: "Bitmap de bloques (ocupado/libre) - Archivo de texto",
      tree: "Estructura jerárquica de directorios y archivos",
      sb: "Información del superbloque (inodos, bloques, espacio libre)",
      file: "Contenido de un archivo específico - Archivo de texto",
      ls: "Listado de archivos y carpetas con permisos y propietarios"
    };
    return descriptions[type] || "Reporte del sistema de archivos";
  };

  const isTextReport = (reportType) => {
    return ['file', 'bm_inode', 'bm_block'].includes(reportType);
  };

  const isNewReport = (report) => {
    return newlyGenerated && newlyGenerated.name === report.name;
  };

  const needsFilePath = (reportType) => {
    return reportType === 'file' || reportType === 'ls';
  };

  return (
    <div style={{ padding: '20px', maxWidth: '1200px', margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
        <h2 style={{ color: '#333', margin: 0 }}>Visor de Reportes ExtreamFS</h2>
        
        <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
          <button 
            onClick={() => fetchReports()} 
            style={{
              padding: '8px 16px',
              backgroundColor: '#6366f1',
              color: 'white',
              border: 'none',
              borderRadius: '6px',
              cursor: 'pointer'
            }}
          >
            Actualizar
          </button>
        </div>
      </div>

      {/* Formulario para generar reportes */}
      <div style={{
        backgroundColor: '#f8fafc',
        padding: '20px',
        borderRadius: '8px',
        marginBottom: '30px',
        border: '1px solid #e2e8f0'
      }}>
        <h3 style={{ margin: '0 0 20px 0', color: '#1e293b' }}>Generar Nuevo Reporte</h3>
        
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '15px', alignItems: 'end' }}>
          <div style={{ minWidth: '200px' }}>
            <label style={{ display: 'block', marginBottom: '5px', fontWeight: '500' }}>Tipo de Reporte:</label>
            {renderReportTypeSelector()}
            <div style={{ fontSize: '12px', color: '#64748b', marginTop: '2px' }}>
              {getReportDescription(formData.reportType)}
            </div>
          </div>
          
          <div style={{ minWidth: '150px' }}>
            <label style={{ display: 'block', marginBottom: '5px', fontWeight: '500' }}>ID de Partición:</label>
            <select 
              name="partitionId" 
              value={formData.partitionId} 
              onChange={handleInputChange}
              style={{
                width: '100%',
                padding: '8px',
                border: '1px solid #cbd5e1',
                borderRadius: '4px',
                backgroundColor: 'white'
              }}
              required
            >
              <option value="">-- Seleccionar --</option>
              {mountedPartitions.map(id => (
                <option key={id} value={id}>{id}</option>
              ))}
            </select>
          </div>
          
          {/* CAMPO CORREGIDO: Mostrar dentro del contenedor principal */}
          {needsFilePath(formData.reportType) && (
            <div style={{ minWidth: '200px' }}>
              <label style={{ display: 'block', marginBottom: '5px', fontWeight: '500' }}>
                {formData.reportType === 'file' ? 'Ruta del archivo:' : 'Ruta del directorio:'}
              </label>
              <input 
                name="filePath" 
                value={formData.filePath} 
                onChange={handleInputChange}
                placeholder={formData.reportType === 'file' ? "/home/archivo.txt" : "/home"}
                style={{
                  width: '100%',
                  padding: '8px',
                  border: '1px solid #cbd5e1',
                  borderRadius: '4px',
                  backgroundColor: 'white'
                }}
                required
              />
            </div>
          )}
          
          <button 
            onClick={generateReport}
            style={{
              padding: '10px 20px',
              backgroundColor: isGenerating ? '#94a3b8' : '#10b981',
              color: 'white',
              border: 'none',
              borderRadius: '6px',
              cursor: isGenerating ? 'not-allowed' : 'pointer',
              fontWeight: '500'
            }}
            disabled={isGenerating}
          >
            {isGenerating ? 'Generando...' : 'Generar Reporte'}
          </button>
        </div>
      </div>

      {/* Indicador de nuevo reporte */}
      {newlyGenerated && (
        <div style={{
          backgroundColor: '#dcfce7',
          border: '1px solid #16a34a',
          borderRadius: '8px',
          padding: '15px',
          marginBottom: '20px',
          display: 'flex',
          alignItems: 'center',
          gap: '10px'
        }}>
          <div style={{ fontSize: '20px' }}>🎉</div>
          <div>
            <strong style={{ color: '#15803d' }}>¡Nuevo reporte generado!</strong>
            <div style={{ fontSize: '14px', color: '#166534' }}>
              {newlyGenerated.name} - {newlyGenerated.type?.toUpperCase()}
            </div>
          </div>
          <button 
            onClick={() => handleViewReport(newlyGenerated)}
            style={{
              marginLeft: 'auto',
              padding: '6px 12px',
              backgroundColor: '#16a34a',
              color: 'white',
              border: 'none',
              borderRadius: '4px',
              cursor: 'pointer'
            }}
          >
            Ver Ahora
          </button>
        </div>
      )}

      {/* Mostrar errores si los hay */}
      {error && (
        <div style={{
          backgroundColor: '#fee2e2',
          border: '1px solid #ef4444',
          borderRadius: '8px',
          padding: '15px',
          color: '#dc2626',
          marginBottom: '20px'
        }}>
          {error}
          <button 
            onClick={() => setError(null)}
            style={{
              marginLeft: '10px',
              backgroundColor: 'transparent',
              border: 'none',
              color: '#dc2626',
              cursor: 'pointer'
            }}
          >
            ✕
          </button>
        </div>
      )}

      {/* Visualizador de reportes */}
      {loading ? (
        <div style={{ textAlign: 'center', padding: '40px', color: '#64748b' }}>
          Cargando reportes disponibles...
        </div>
      ) : (
        <div>
          {Object.keys(reports).length === 0 ? (
            <div style={{
              textAlign: 'center',
              padding: '60px 20px',
              backgroundColor: '#f1f5f9',
              borderRadius: '8px',
              color: '#64748b'
            }}>
              <div style={{ fontSize: '48px', marginBottom: '10px' }}>📊</div>
              <h3 style={{ margin: '0 0 10px 0' }}>No hay reportes disponibles</h3>
              <p style={{ margin: 0 }}>Genere uno usando el formulario de arriba</p>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '30px' }}>
              {Object.entries(reports).map(([type, typeReports]) => (
                
                <div key={type}>
                  <div style={{ 
                    display: 'flex', 
                    alignItems: 'center', 
                    gap: '10px', 
                    marginBottom: '15px',
                    paddingBottom: '10px',
                    borderBottom: '2px solid #e2e8f0'
                  }}>
                    <h3 style={{ margin: 0, color: '#1e293b' }}>
                      {type.toUpperCase()} Reports
                    </h3>
                    <span style={{
                      backgroundColor: '#6366f1',
                      color: 'white',
                      padding: '4px 8px',
                      borderRadius: '12px',
                      fontSize: '12px',
                      fontWeight: '500'
                    }}>
                      {typeReports.length}
                    </span>
                    <span style={{ fontSize: '14px', color: '#64748b' }}>
                      {getReportDescription(type)}
                    </span>
                  </div>
                  
                  <div style={{
                    display: 'grid',
                    gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
                    gap: '20px'
                  }}>
                    {typeReports.map((report, index) => (
                      <div 
                        key={index} 
                        onClick={() => handleViewReport(report)}
                        style={{
                          border: isNewReport(report) ? '3px solid #16a34a' : '1px solid #e2e8f0',
                          borderRadius: '12px',
                          overflow: 'hidden',
                          cursor: 'pointer',
                          transition: 'all 0.2s ease',
                          backgroundColor: isNewReport(report) ? '#f0fdf4' : 'white',
                          boxShadow: isNewReport(report) ? '0 4px 12px rgba(34, 197, 94, 0.15)' : '0 2px 4px rgba(0,0,0,0.1)',
                          transform: 'scale(1)',
                          position: 'relative'
                        }}
                        onMouseEnter={(e) => {
                          e.currentTarget.style.transform = 'scale(1.02)';
                          e.currentTarget.style.boxShadow = '0 8px 25px rgba(0,0,0,0.15)';
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.transform = 'scale(1)';
                          e.currentTarget.style.boxShadow = isNewReport(report) ? '0 4px 12px rgba(34, 197, 94, 0.15)' : '0 2px 4px rgba(0,0,0,0.1)';
                        }}
                      >
                        {isNewReport(report) && (
                          <div style={{
                            position: 'absolute',
                            top: '10px',
                            right: '10px',
                            backgroundColor: '#16a34a',
                            color: 'white',
                            padding: '4px 8px',
                            borderRadius: '12px',
                            fontSize: '12px',
                            fontWeight: '500',
                            zIndex: 1
                          }}>
                            NUEVO
                          </div>
                        )}
                        
                        <div style={{ height: 'auto', maxHeight: '400px', overflow: 'auto', backgroundColor: '#f8fafc', paddingTop: '10px' }}>
                          {report.extension === '.txt' || isTextReport(type) ? (
                            <div style={{
                              padding: '15px',
                              backgroundColor: 'white',
                              margin: '10px',
                              borderRadius: '6px',
                              border: '1px solid #e2e8f0',
                              fontFamily: 'monospace',
                              fontSize: '12px',
                              lineHeight: '1.4',
                              color: '#1e293b',
                              maxHeight: '300px',
                              overflow: 'auto'
                            }}>
                              <div style={{ marginBottom: '10px', fontWeight: 'bold', color: '#6366f1' }}>
                                📄 Contenido del archivo de texto
                              </div>
                              <div style={{ 
                                whiteSpace: 'pre-wrap',
                                wordBreak: 'break-word'
                              }}>
                                Haga clic para ver el contenido completo
                              </div>
                            </div>
                          ) : (
                            <>
                              <img
                                src={`http://localhost:8080${report.url}`}
                                alt={report.name}
                                style={{
                                  width: '100%',
                                  height: 'auto',
                                  objectFit: 'contain',
                                  objectPosition: 'top',
                                  backgroundColor: 'white'
                                }}
                                onLoad={(e) => {
                                  console.log(`Imagen cargada exitosamente: ${report.name}`);
                                }}
                                onError={(e) => {
                                  console.error(`Error cargando imagen: ${report.name}, URL: ${e.target.src}`);
                                  e.target.style.display = 'none';
                                  e.target.nextSibling.style.display = 'flex';
                                }}
                              />
                              <div style={{
                                display: 'none',
                                height: '200px',
                                alignItems: 'center',
                                justifyContent: 'center',
                                color: '#64748b',
                                fontSize: '14px',
                                backgroundColor: '#f1f5f9'
                              }}>
                                ❌ Error cargando imagen
                              </div>
                            </>
                          )}
                        </div>
                        
                        <div style={{ padding: '15px' }}>
                          <div style={{ 
                            fontWeight: '500', 
                            marginBottom: '5px',
                            color: '#1e293b',
                            fontSize: '14px'
                          }}>
                            {report.name}
                          </div>
                          <div style={{ 
                            fontSize: '12px', 
                            color: '#64748b',
                            marginBottom: '8px'
                          }}>
                            {report.created}
                          </div>
                          <div style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            alignItems: 'center'
                          }}>
                            <span style={{
                              backgroundColor: '#e2e8f0',
                              padding: '2px 6px',
                              borderRadius: '4px',
                              fontSize: '11px',
                              color: '#475569'
                            }}>
                              {report.size}
                            </span>
                            <span style={{
                              backgroundColor: '#ddd6fe',
                              padding: '2px 6px',
                              borderRadius: '4px',
                              fontSize: '11px',
                              color: '#5b21b6',
                              fontWeight: '500'
                            }}>
                              {type.toUpperCase()}
                            </span>
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {selectedReport && (
        <div
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundColor: 'rgba(0, 0, 0, 0.75)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 1000,
            padding: '20px',
          }}
          onClick={handleCloseViewer}
        >
          <div
            style={{
              backgroundColor: 'white',
              borderRadius: '12px',
              maxWidth: '90vw',
              maxHeight: '90vh',
              overflow: 'auto',
              display: 'flex',
              flexDirection: 'column',
              padding: '10px',
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div
              style={{
                padding: '10px 20px',
                borderBottom: '1px solid #e2e8f0',
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
              }}
            >
              <div>
                <h3 style={{ margin: '0 0 5px 0', color: '#1e293b' }}>{selectedReport.name}</h3>
                <p style={{ margin: 0, fontSize: '14px', color: '#64748b' }}>
                  Generado: {selectedReport.created}
                </p>
              </div>
              <button
                onClick={handleCloseViewer}
                style={{
                  background: 'none',
                  border: 'none',
                  fontSize: '24px',
                  cursor: 'pointer',
                  color: '#64748b',
                  padding: '5px',
                }}
              >
                ×
              </button>
            </div>

            <div
              style={{
                flex: 1,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                padding: '20px',
                backgroundColor: '#f8fafc',
              }}
            >
              {selectedReport && (selectedReport.extension === '.txt' || 
                  ['bm_inode', 'bm_block', 'file'].includes(selectedReport.type)) ? (
                  <iframe
                    src={`http://localhost:8080${selectedReport.url}`}
                    title={selectedReport.name}
                    style={{
                      width: '100%',
                      height: '500px',
                      border: '1px solid #e2e8f0',
                      borderRadius: '8px',
                      backgroundColor: 'white',
                    }}
                    onLoad={(e) => {
                      console.log(`Archivo de texto cargado: ${selectedReport.name}`);
                    }}
                    onError={(e) => {
                      console.error(`Error cargando archivo de texto: ${selectedReport.name}`);
                    }}
                  />
              ) : (
                <img
                  src={`http://localhost:8080${selectedReport.url}`}
                  alt={selectedReport.name}
                  style={{
                    width: '100%',
                    height: 'auto',
                    objectFit: 'contain',
                    objectPosition: 'top',
                    backgroundColor: 'white'
                  }}
                  onLoad={(e) => {
                    console.log(`Imagen cargada exitosamente: ${selectedReport.name}`);
                  }}
                  onError={(e) => {
                    console.error(`Error cargando imagen: ${selectedReport.name}, URL: ${e.target.src}`);
                  }}
                />
              )}
            </div>

            <div
              style={{
                padding: '10px 20px',
                borderTop: '1px solid #e2e8f0',
                display: 'flex',
                gap: '10px',
                justifyContent: 'flex-end',
              }}
            >
              <a
                href={`http://localhost:8080${selectedReport.url}`}
                download={selectedReport.name}
                style={{
                  padding: '8px 16px',
                  backgroundColor: '#3b82f6',
                  color: 'white',
                  textDecoration: 'none',
                  borderRadius: '6px',
                  fontSize: '14px',
                }}
              >
                Descargar {(selectedReport.extension === '.txt' || 
                          ['bm_inode', 'bm_block', 'file', 'ls'].includes(selectedReport.type)) 
                          ? 'Archivo' : 'Imagen'}
              </a>
              {selectedReport.extension !== '.txt' && (
                <a
                  href={`http://localhost:8080/reports/${selectedReport.name.replace(/\.[^.]+$/, '.dot')}`}
                  download={selectedReport.name.replace(/\.[^.]+$/, '.dot')}
                  style={{
                    padding: '8px 16px',
                    backgroundColor: '#64748b',
                    color: 'white',
                    textDecoration: 'none',
                    borderRadius: '6px',
                    fontSize: '14px',
                  }}
                >
                  Descargar DOT
                </a>
              )}
            </div>
          </div>
        </div>
      )}

      <style>{`
        .report-selector {
          width: 100%;
          padding: 8px;
          border: 1px solid #cbd5e1;
          border-radius: 4px;
          background-color: white;
          font-size: 14px;
        }
        
        .report-selector:focus {
          outline: none;
          border-color: #6366f1;
          box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
        }
      `}</style>
    </div>
  );
};

export default ReportsViewer;