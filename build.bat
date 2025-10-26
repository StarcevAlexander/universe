@echo off
echo ========================================
echo    Go + Angular Build Script
echo ========================================

echo Building frontend...
cd frontend
if errorlevel 1 (
    echo ERROR: Frontend folder not found!
    goto error
)
call npm run build
if errorlevel 1 (
    echo ERROR: Frontend build failed!
    goto error
)

echo Building backend...
cd ../backend
if errorlevel 1 (
    echo ERROR: Backend folder not found!
    goto error
)
go build -o app.exe
if errorlevel 1 (
    echo ERROR: Backend build failed!
    goto error
)

echo.
echo ========================================
echo    BUILD COMPLETED SUCCESSFULLY!
echo ========================================
echo.
echo Files created:
echo - frontend\dist\browser\ (Angular files)
echo - backend\app.exe (Go server)
echo.
echo To start server: cd backend && app.exe
echo.
echo Window will close in 15 seconds...
timeout /t 15
exit /b 0

:error
echo.
echo ========================================
echo    BUILD FAILED!
echo ========================================
echo.
echo Window will close in 15 seconds...
timeout /t 15
exit /b 1