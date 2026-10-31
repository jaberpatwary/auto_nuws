.DEFAULT_GOAL := help
.PHONY: help start stop

help:
	@echo Available commands: make start, make stop

start:
	powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/start.ps1

stop:
	powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/stop.ps1
