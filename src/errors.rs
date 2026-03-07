use miette::Diagnostic;
use thiserror::Error;

#[allow(dead_code)]
#[derive(Error, Diagnostic, Debug)]
pub enum BackscrollError {
    #[error("Error al abrir la base de datos: {0}")]
    #[diagnostic(
        code(backscroll::db_open_error),
        help("Verifica los permisos del archivo o el espacio en disco.")
    )]
    DatabaseOpen(String),

    #[error("Error al parsear la sesión: {0}")]
    #[diagnostic(
        code(backscroll::parse_error),
        help("El archivo de sesión puede estar corrupto o usar un formato no soportado.")
    )]
    ParseError(String),

    #[error("No se encontró el directorio de sesiones: {path}")]
    #[diagnostic(
        code(backscroll::io_error),
        help("Asegúrate de que la ruta sea correcta.")
    )]
    PathNotFound { path: String },
}
