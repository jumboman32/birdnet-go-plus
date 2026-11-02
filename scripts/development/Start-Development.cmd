@echo off
cd /d "%~dp0"
node "%~dp0start-development.mjs"
if errorlevel 1 pause
