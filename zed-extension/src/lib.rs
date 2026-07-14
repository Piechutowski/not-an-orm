//! Zed glue for the EDBML language server.
//!
//! The extension's only job is to tell Zed how to launch `edbml-ls`. The
//! binary is resolved in order: the user's `lsp.edbml-ls.binary.path`
//! setting, then the worktree `PATH`. The extension is installed locally as
//! a dev extension, so there is no download fallback — build the server
//! with `go install ./cmd/edbml-ls` instead.

use zed_extension_api::{self as zed, settings::LspSettings, LanguageServerId, Result};

struct EdbmlExtension;

impl zed::Extension for EdbmlExtension {
    fn new() -> Self {
        Self
    }

    fn language_server_command(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<zed::Command> {
        let binary_settings = LspSettings::for_worktree(language_server_id.as_ref(), worktree)
            .ok()
            .and_then(|settings| settings.binary);

        if let Some(path) = binary_settings.as_ref().and_then(|binary| binary.path.clone()) {
            return Ok(zed::Command {
                command: path,
                args: binary_settings
                    .and_then(|binary| binary.arguments)
                    .unwrap_or_default(),
                env: Default::default(),
            });
        }

        let path = worktree.which("edbml-ls").ok_or_else(|| {
            "edbml-ls not found on PATH. Build it with `go install ./cmd/edbml-ls`, or point \
             Zed at the binary via the `lsp.edbml-ls.binary.path` setting."
                .to_string()
        })?;

        Ok(zed::Command {
            command: path,
            args: Vec::new(),
            env: worktree.shell_env(),
        })
    }

    fn language_server_initialization_options(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<Option<zed::serde_json::Value>> {
        Ok(LspSettings::for_worktree(language_server_id.as_ref(), worktree)
            .ok()
            .and_then(|settings| settings.initialization_options))
    }

    fn language_server_workspace_configuration(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<Option<zed::serde_json::Value>> {
        Ok(LspSettings::for_worktree(language_server_id.as_ref(), worktree)
            .ok()
            .and_then(|settings| settings.settings))
    }
}

zed::register_extension!(EdbmlExtension);
