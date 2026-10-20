$ErrorActionPreference = 'Stop'
$serviceRoot = Split-Path -Parent $PSScriptRoot
$repoRoot = Split-Path -Parent $serviceRoot
$pythonExe = Join-Path $serviceRoot '.venv/Scripts/python.exe'
$goBin = Join-Path $serviceRoot '.tools'
$goOutput = Join-Path $repoRoot 'pkg/ragapi'
New-Item -ItemType Directory -Force -Path $goOutput | Out-Null
if (!(Test-Path (Join-Path $goBin 'protoc-gen-go.exe')) -or !(Test-Path (Join-Path $goBin 'protoc-gen-go-grpc.exe'))) {
    throw 'Install the pinned Go generators into rag/.tools first; see rag/README.md.'
}
& $pythonExe -m grpc_tools.protoc "-I$repoRoot/proto" "--python_out=$serviceRoot/src" "--grpc_python_out=$serviceRoot/src" `
    "--plugin=protoc-gen-go=$goBin/protoc-gen-go.exe" "--plugin=protoc-gen-go-grpc=$goBin/protoc-gen-go-grpc.exe" `
    "--go_out=$goOutput" --go_opt=paths=source_relative "--go-grpc_out=$goOutput" --go-grpc_opt=paths=source_relative `
    "$repoRoot/proto/rag/v1/rag.proto"
if ($LASTEXITCODE -ne 0) { throw 'Protobuf generation failed' }
