#Requires -RunAsAdministrator
param([string]$ServerIP = '203.0.113.10')
route delete $ServerIP
Write-Host "Removed bypass route for $ServerIP"
