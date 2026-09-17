@echo off
setlocal
cd /d "%~dp0"
if not defined BASE_FRONTEND_DIR set "BASE_FRONTEND_DIR=%CD%\frontend\dist"
if not defined BASE_DATA_DIR set "BASE_DATA_DIR=%CD%\data"
baseplatform.exe
exit /b %errorlevel%
