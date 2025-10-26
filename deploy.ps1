# Сборка
.\build.bat

# Загрузка на VM
$VM_USER = "deploy"
$VM_HOST = "your-vm-ip"
$VM_PATH = "/opt/your-app"

# Копируем фронтенд
scp -r .\frontend\dist\browser\* ${VM_USER}@${VM_HOST}:${VM_PATH}/static/

# Копируем бэкенд
scp .\backend\app.exe ${VM_USER}@${VM_HOST}:${VM_PATH}/app

Write-Host "Deployment completed!" -ForegroundColor Green