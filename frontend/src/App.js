import React, { useState } from 'react';
import './app.css';
import CommandInput from './components/CommandInput';
import CommandOutput from './components/CommandOutput';
import FileUploader from './components/FileUploader';
import ReportsViewer from './components/ReportsViewer';
import Login from './components/Login';
import FileSystemExplorer from './components/FileSystemExplorer';
import JournalingViewer from './components/JournalingViewer';
import './components/ReportsViewer.css';

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
      // Ejecutar comando login en el backend
      const loginCommand = `login -user="${user}" -pass="${pass}" -id="${id}"`;
      const resp = await fetch('http://18.222.221.137:8080/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ commands: loginCommand })
      });

      if (!resp.ok) {
        setLoginError('Error del servidor');
        return;
      }

      const json = await resp.json();
      
      // Verificar si el login fue exitoso
      if (json.output && json.output.includes('SESIÓN INICIADA EXITOSAMENTE')) {
        setSession({ user, id });
        setIsLoginOpen(false);
        setActiveTab('explorer'); // Cambiar a la pestaña del explorador
        setOutput(json.output); // Mostrar el output del login
      } else {
        // Extraer el mensaje de error del output
        const errorMatch = json.output.match(/Error[^:]*:\s*(.+)/i);
        setLoginError(errorMatch ? errorMatch[1] : 'Credenciales incorrectas');
      }
    } catch (err) {
      setLoginError('Error de conexión con el servidor');
    } finally {
      setLoginLoading(false);
    }
  };

  

  const handleLogout = async () => {
    try {
      // Ejecutar comando logout en el backend
      const resp = await fetch('http://18.222.221.137:8080/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ commands: 'logout' })
      });

      if (resp.ok) {
        const json = await resp.json();
        setOutput(json.output);
      }
    } catch (err) {
      console.error('Error al hacer logout:', err);
    } finally {
      setSession(null);
      setActiveTab('commands'); // Volver a la pestaña de comandos
    }
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
      const resp = await fetch('http://18.222.221.137:8080/execute', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ commands })
      });

      if (!resp.ok) {
        setOutput(`Error del servidor: ${resp.status}`);
        return;
      }

      const json = await resp.json();
      if (json.output) {
        setOutput(json.output);
      } else if (json.error) {
        setOutput(`Error: ${json.error}`);
      } else {
        setOutput('(sin salida)');
      }
    } catch (err) {
      setOutput(`Error de conexión: ${err.message}\nVerifica que el servidor esté ejecutándose en http://18.222.221.137:8080`);
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
          <div className="session-info">
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
          {session && (
            <button 
              className={`nav-tab ${activeTab === 'explorer' ? 'active' : ''}`}
              onClick={() => setActiveTab('explorer')}
            >
              📁 Explorador
            </button>
          )}
          {session && (
            <button 
              className={`nav-tab ${activeTab === 'journaling' ? 'active' : ''}`}
              onClick={() => setActiveTab('journaling')}
            >
              📜 Journaling
            </button>
          )}
        </nav>
      </header>

      <main className="App-main">
        {activeTab === 'commands' && (
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
        )}

        {activeTab === 'reports' && <ReportsViewer />}

        {activeTab === 'explorer' && session && (
          <FileSystemExplorer 
            session={session}
            onExecuteCommand={executeCommands}
            setOutput={setOutput}
          />
        )}
        {activeTab === 'journaling' && session && (
          <JournalingViewer session={session} />
        )}
      </main>
    </div>
  );
}

export default App;