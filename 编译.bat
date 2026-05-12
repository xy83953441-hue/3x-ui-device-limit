@echo off
cd /d "%~dp0"
chcp 65001 >nul
echo ================================
echo  正在下载 Go 依赖...
echo ================================
go mod download

echo ================================
echo  正在编译 x-ui (amd64)...
echo ================================
go build -o x-ui.exe -ldflags "-s -w" .

echo ================================
echo  正在打包...
echo ================================
mkdir dist 2>nul
cd dist
mkdir x-ui 2>nul
cd x-ui
copy ..\..\x-ui.exe . >nul
copy ..\..\x-ui.sh . >nul
copy ..\..\x-ui.service.debian . >nul
copy ..\..\x-ui.service.rhel . >nul
copy ..\..\x-ui.service.arch . >nul
copy ..\..\x-ui.rc . >nul
mkdir bin 2>nul

cd ..

echo ================================
echo  创建压缩包...
echo ================================
powershell -command "Compress-Archive -Path 'x-ui' -DestinationPath 'x-ui-linux-amd64.tar.gz' -Force"
del x-ui.exe /f /q 2>nul
rmdir /s /q x-ui 2>nul
cd ..

echo ================================
echo  编译完成！
echo  文件在: %~dp0dist\x-ui-linux-amd64.tar.gz
echo ================================
echo.
echo  请将 dist\x-ui-linux-amd64.tar.gz 上传到 GitHub Release
echo.
pause
