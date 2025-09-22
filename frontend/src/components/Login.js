import React, { useState } from 'react';

const Login = ({ isOpen, onClose, onLogin, isLoading, errorMsg }) => {
  const [user, setUser] = useState('');
  const [pass, setPass] = useState('');
  const [id, setId] = useState('');
  const [showPass, setShowPass] = useState(false);
  const [closing, setClosing] = useState(false);

  const handleSubmit = (e) => {
    e.preventDefault();
    onLogin({ user, pass, id });
  };

  // Maneja el cierre animado
  const handleClose = () => {
    setClosing(true);
    setTimeout(() => {
      setClosing(false);
      onClose();
      setUser('');
      setPass('');
      setId('');
    }, 350); // Debe coincidir con la duración de la animación
  };

  if (!isOpen && !closing) return null;

  return (
    <div className={`modal-overlay${closing ? ' closing' : ''}`}>
      <div className={`modal-content${closing ? ' closing' : ''}`}>
        <h2>Iniciar Sesión</h2>
        <form onSubmit={handleSubmit}>
          <input
            type="text"
            placeholder="Usuario"
            value={user}
            onChange={e => setUser(e.target.value)}
            required
            autoFocus
          />
          <div style={{
            display: 'flex',
            alignItems: 'center',
            background: '#f1f5f9',
            border: '1.5px solid #c7d2fe',
            borderRadius: 8,
            marginBottom: '0.7rem',
            padding: 0,
            position: 'relative'
            }}>
            <input
            className="password-input"
            type={showPass ? "text" : "password"}
            placeholder="Contraseña"
            value={pass}
            onChange={e => setPass(e.target.value)}
            required
            // ...sin estilos inline de border/background/padding
            />
            <button
                type="button"
                className="showpass-btn"
                onClick={() => setShowPass(v => !v)}
                tabIndex={-1}
                aria-label="Mostrar/Ocultar contraseña"
                style={{
                position: 'absolute',
                right: 10,
                top: '50%',
                transform: 'translateY(-50%)',
                background: 'none',
                border: 'none',
                fontSize: '1.2rem',
                cursor: 'pointer',
                color: '#6366f1',
                padding: 0
                }}
            >
                {showPass ? "🙈" : "👁️"}
            </button>
            </div>
          <input
            type="text"
            placeholder="ID de Partición"
            value={id}
            onChange={e => setId(e.target.value)}
            required
          />
          {errorMsg && <div className="login-error">{errorMsg}</div>}
          <button
            type="submit"
            disabled={!user || !pass || !id || isLoading}
          >
            {isLoading ? "Validando..." : "Entrar"}
          </button>
          <button type="button" onClick={handleClose} disabled={isLoading}>Cancelar</button>
        </form>
      </div>
    </div>
  );
};

export default Login;