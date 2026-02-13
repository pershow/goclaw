@echo off
REM Build script for GoClaw Control UI (Windows)

echo Building GoClaw Control UI...

REM Build UI
echo Building frontend...
cd ui
call npm run build
if %errorlevel% neq 0 exit /b %errorlevel%
cd ..

REM Copy to gateway
echo Copying to gateway...
if exist gateway\dist\control-ui rmdir /s /q gateway\dist\control-ui
if not exist gateway\dist mkdir gateway\dist
xcopy /E /I /Y dist\control-ui gateway\dist\control-ui

REM Build Go binary
echo Building Go binary...
go build -o goclaw.exe .
if %errorlevel% neq 0 exit /b %errorlevel%

echo.
echo Build complete!
echo.
echo To run the gateway with UI:
echo   goclaw.exe gateway run --port 28789
echo.
echo Then open: http://localhost:28789/
