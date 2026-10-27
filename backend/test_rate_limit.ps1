param(
    [Parameter(Mandatory = $true)]
    [string]$ApiKey,

    [int]$Count = 30,

    [string]$BaseUrl = "http://localhost:8080",

    [int]$ThrottleLimit = 100
)

if ($PSVersionTable.PSVersion.Major -lt 7) {
    throw "This script requires PowerShell 7 or newer because it uses ForEach-Object -Parallel."
}

if ($Count -lt 1) {
    throw "Count must be at least 1."
}

$jobs = 1..$Count | ForEach-Object -Parallel {
    $number = $_
    $uri = "$using:BaseUrl/jobs"
    $headers = @{ "X-API-Key" = $using:ApiKey }
    $body = @{ type = "addition"; a = $number; b = 1 } | ConvertTo-Json -Compress

    try {
        $response = Invoke-WebRequest `
            -Uri $uri `
            -Method Post `
            -Headers $headers `
            -ContentType "application/json" `
            -Body $body `
            -UseBasicParsing

        [pscustomobject]@{
            Number     = $number
            StatusCode = [int]$response.StatusCode
            Result     = $response.Content
        }
    }
    catch {
        $statusCode = 0
        if ($_.Exception.Response) {
            $statusCode = [int]$_.Exception.Response.StatusCode
        }

        [pscustomobject]@{
            Number     = $number
            StatusCode = $statusCode
            Result     = $_.Exception.Message
        }
    }
} -ThrottleLimit $ThrottleLimit

$summary = $jobs | Group-Object StatusCode | Sort-Object Name
$summary | Select-Object Name, Count | Format-Table -AutoSize

Write-Host "Total requests: $Count"
Write-Host "Accepted (201): $(@($jobs | Where-Object StatusCode -eq 201).Count)"
Write-Host "Rate limited (429): $(@($jobs | Where-Object StatusCode -eq 429).Count)"
Write-Host "Other responses: $(@($jobs | Where-Object { $_.StatusCode -ne 201 -and $_.StatusCode -ne 429 }).Count)"

$jobs |
    Where-Object { $_.StatusCode -ne 201 } |
    Select-Object Number, StatusCode, Result |
    Format-Table -Wrap -AutoSize
