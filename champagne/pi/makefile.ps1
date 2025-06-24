# PowerShell equivalent of makefile
# Builds Go binary for Linux ARM and creates tar.gz archive

param(
    [string]$Target = "all"
)

function Build-Pi {
    Write-Host "Building pi binary for Linux ARM..."
    
    # Save current environment variables
    $originalGOOS = $env:GOOS
    $originalGOARCH = $env:GOARCH
    
    # Prepare Go environment and dependencies
    Write-Host "Running go mod tidy..."
    go mod tidy
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error "go mod tidy failed"
        exit 1
    }
    
    Write-Host "Running go mod download..."
    go mod download
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error "go mod download failed"
        exit 1
    }
    
    Write-Host "Running go vet..."
    go vet ./...
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error "go vet found issues"
        exit 1
    }
    
    # Set environment variables for cross-compilation
    $env:GOOS = "linux"
    $env:GOARCH = "arm"
    # Build the Go binary
    go build -o pi .
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Go build failed"
        exit 1
    }
    
    Write-Host "Creating pi.tar.gz archive..."
    
    # Create tar.gz archive (requires tar to be available on Windows)
    tar -czvf pi.tar.gz pi templates public
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to create tar.gz archive"
        exit 1
    }
    Write-Host "Cleaning up pi binary..."
    
    # Remove the binary file
    if (Test-Path "pi") {
        Remove-Item "pi" -Force
    }
    # Restore original environment variables
    if ($null -eq $originalGOOS) {
        $env:GOOS = "windows"
    }
    else {
        $env:GOOS = $originalGOOS
    }
    
    if ($null -eq $originalGOARCH) {
        $env:GOARCH = "amd64"
    }
    else {
        $env:GOARCH = $originalGOARCH
    }
    
    Write-Host "Build complete: pi.tar.gz created successfully"
}

function Show-Help {
    Write-Host "Usage: .\makefile.ps1 [target]"
    Write-Host ""
    Write-Host "Targets:"
    Write-Host "  all        Build pi.tar.gz (default)"
    Write-Host "  pi.tar.gz  Build pi.tar.gz"
    Write-Host "  help       Show this help message"
}

# Main script logic
switch ($Target.ToLower()) {
    "all" { Build-Pi }
    "pi.tar.gz" { Build-Pi }
    "help" { Show-Help }
    default { 
        Write-Host "Unknown target: $Target"
        Show-Help
        exit 1
    }
}
