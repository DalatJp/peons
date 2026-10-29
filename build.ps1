# Construit l'exécutable unique : peons.exe (Windows).
#
#   .\build.ps1            construit dans le dossier courant
#   .\build.ps1 -Out C:\Outils\peons.exe
#   .\build.ps1 -SignWith <empreinte>   signe l'exe avec ce certificat (Cert:\CurrentUser\My)
#
# -H=windowsgui : sous-système graphique, un double-clic n'ouvre pas de console.
# Les sprites et la police sont embarqués dans l'exe, qui n'a aucune autre dépendance
# que les DLL de Windows : c'est le seul fichier à déployer.
# -SignWith : appose une signature Authenticode après le build (certificat de
# signature de code, par empreinte). Tant que le certificat n'est pas de confiance
# sur la machine, la signature est « UnknownError » (signée mais éditeur non
# approuvé) ; elle devient « Valid » une fois l'éditeur ajouté aux éditeurs de
# confiance. Horodatage best-effort : s'il est injoignable (proxy), on signe sans.
param([string]$Out = "", [string]$SignWith = "")

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
if (-not $Out) { $Out = Join-Path $PSScriptRoot "peons.exe" }

$env:CGO_ENABLED = "0"
go test ./... | Select-String -NotMatch "no test files"
if ($LASTEXITCODE -ne 0) { throw "les tests échouent" }
go build -trimpath -ldflags "-s -w -H=windowsgui" -o $Out ./cmd/peons
if ($LASTEXITCODE -ne 0) { throw "la compilation échoue" }
if ($SignWith) {
    $cert = Get-Item "Cert:\CurrentUser\My\$SignWith" -ErrorAction Stop
    # d'abord avec horodatage ; si la signature n'est pas posée (TSA injoignable), sans
    $null = Set-AuthenticodeSignature -FilePath $Out -Certificate $cert -HashAlgorithm SHA256 `
        -TimestampServer "http://timestamp.digicert.com" -ErrorAction SilentlyContinue
    if (-not (Get-AuthenticodeSignature $Out).SignerCertificate) {
        $null = Set-AuthenticodeSignature -FilePath $Out -Certificate $cert -HashAlgorithm SHA256
    }
    $sig = Get-AuthenticodeSignature $Out
    if (-not $sig.SignerCertificate) { throw "la signature a échoué" }
    "signé : statut {0}, horodaté {1}" -f $sig.Status, [bool]$sig.TimeStamperCertificate
}
"{0} ({1:N1} Mo)" -f $Out, ((Get-Item $Out).Length / 1MB)
