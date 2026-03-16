@echo off
REM Run decrypt then sync (daily scheduled task).
REM Place this script in the script/ folder; chatlog.exe should be in the parent directory.
REM CHATLOG_DIR must point to your config dir (~/.chatlog) so decrypt/sync use the same config as the TUI.
set "SCRIPT_DIR=%~dp0"
set "CHATLOG_ROOT=%SCRIPT_DIR%.."
set "CHATLOG_DIR=%USERPROFILE%\.chatlog"
cd /d "%CHATLOG_ROOT%"
"%CHATLOG_ROOT%\chatlog.exe" decrypt && "%CHATLOG_ROOT%\chatlog.exe" sync
