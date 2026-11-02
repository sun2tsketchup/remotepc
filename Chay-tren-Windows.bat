@echo off
chcp 65001 >nul
cd /d "%~dp0agent"
where python >nul 2>nul || (echo Chua co Python. Tai tai https://www.python.org/downloads/ - nho tick "Add python.exe to PATH" & pause & exit /b)
if not exist .venv (python -m venv .venv)
call .venv\Scripts\activate.bat
pip install -q -r requirements.txt
python server.py %*
pause
