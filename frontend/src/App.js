import React, { useState } from 'react';
import './app.css';
import CommandInput from './components/CommandInput';
import CommandOutput from './components/CommandOutput';
import FileUploader from './components/FileUploader';
import ReportsViewer from './components/ReportsViewer';
import Login from './components/Login'; // Importar el componente de Login
import './components/ReportsViewer.css'; // Importar los estilos

function App() {
  const [commands, setCommands] = useState('');
  const [output, setOutput] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [activeTab, setActiveTab] = useState('commands');
  const [isLoginOpen, setIsLoginOpen] = useState(false);
  const [session, setSession] = useState(null);
  const [loginLoading, setLoginLoading] = useState(false);
  const [loginError, setLoginError] = useState('');

const handleLogin = async ({ user, pass, id }) => {
  setLoginLoading(true);
  setLoginError('');
  try {
    // Aquí deberías hacer la petición real al backend
    const resp = await fetch('http://localhost:8080/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ user, pass, id })
    });
    const json = await resp.json();
    if (resp.ok && json.success) {
      setSession({ user, id });
      setIsLoginOpen(false);
    } else {
      setLoginError(json.error || 'Credenciales incorrectas');
    }
  } catch (err) {
    setLoginError('Error de conexión');
  } finally {
    setLoginLoading(false);
  }
};
const handleLogout = () => {
  setSession(null);
};


  const handleFileLoad = (fileContent) => {
    setCommands(fileContent);
  };


  const executeCommands = async () => {
    if (!commands.trim()) {
      setOutput('No hay comandos para ejecutar.');
      return;
    }

    setIsLoading(true);
    setOutput('');

    try {
      const resp = await fetch('http://localhost:8080/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ commands }) // enviar TODO el script (comentarios incluidos)
      });

      if (!resp.ok) {
        setOutput(`Error del servidor: ${resp.status}`);
        return;
      }

      const json = await resp.json();
      if (json.output) {
        // Mostrar la salida completa generada por el servidor (mantiene comentarios y resumen)
        setOutput(json.output);
      } else if (json.error) {
        setOutput(`Error: ${json.error}`);
      } else {
        setOutput('(sin salida)');
      }
    } catch (err) {
      setOutput(`Error de conexión: ${err.message}\nVerifica que el servidor esté ejecutándose en http://localhost:8080`);
    } finally {
      setIsLoading(false);
    }
  };


  const clearCommands = () => {
    setCommands('');
  };

  const clearOutput = () => {
    setOutput('');
  };

  return (
    <div className="App">
      <header className="App-header">
        <h1>ExtreamFS</h1>
        <p>MIA - Sistema de Archivos</p>
        {!session ? (
        <button className="btn-primary" onClick={() => setIsLoginOpen(true)}>
          Iniciar Sesión
        </button>
      ) : (
        <div>
          <span>Sesión: {session.user} ({session.id})</span>
          <button className="btn-secondary" onClick={handleLogout}>Cerrar sesión</button>
        </div>
      )}
      <Login
        isOpen={isLoginOpen}
        onClose={() => setIsLoginOpen(false)}
        onLogin={handleLogin}
        isLoading={loginLoading}
        errorMsg={loginError}
      />
        {/* Menú de navegación entre pestañas */}
        <nav className="app-nav">
          <button 
            className={`nav-tab ${activeTab === 'commands' ? 'active' : ''}`}
            onClick={() => setActiveTab('commands')}
          >
            Comandos
          </button>
          <button 
            className={`nav-tab ${activeTab === 'reports' ? 'active' : ''}`}
            onClick={() => setActiveTab('reports')}
          >
            Reportes
          </button>
        </nav>
      </header>

      <main className="App-main">
        {activeTab === 'commands' ? (
          <div className="container">
            <div className="input-section">
              <div className="section-header">
                <h2>Área de Entrada de Comandos</h2>
                <div className="button-group">
                  <FileUploader onFileLoad={handleFileLoad} />
                  <button onClick={clearCommands} className="btn-secondary">
                    Limpiar
                  </button>
                </div>
              </div>
              
              <CommandInput 
                value={commands}
                onChange={setCommands}
                placeholder="Ingresa los comandos aquí, uno por línea:

Ejemplo:
mkdisk -size=3000 -unit=k -path=disk1.mia
mkdisk -size=5000 -unit=m -path=disk2.mia
fdisk -size=1000 -path=disk1.mia -name=part1

# Los comentarios empiezan con #
# Cada línea es un comando separado"
              />
              
              <div className="execute-section">
                <button 
                  onClick={executeCommands}
                  disabled={isLoading || !commands.trim()}
                  className="btn-execute"
                >
                  {isLoading ? 'Ejecutando...' : 'Ejecutar Comandos'}
                </button>
              </div>
            </div>

            <div className="output-section">
              <div className="section-header">
                <h2>Área de Salida de Comandos</h2>
                <button onClick={clearOutput} className="btn-secondary">
                  Limpiar Salida
                </button>
              </div>
              
              <CommandOutput 
                output={output}
                isLoading={isLoading}
              />
            </div>
          </div>
        ) : (
          <ReportsViewer />
        )}
      </main>
    </div>
  );
}

export default App;