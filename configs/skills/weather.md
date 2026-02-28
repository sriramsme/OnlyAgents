---
name: weather
description: Get current weather, forecasts, and historical weather data
version: 1.0.0
capabilities:
  - weather
authors:
  - name: John Doe
    email: john@example.com
homepage: https://github.com/user/weather-skill
requires:
  bins:
    - curl
  env: []
security:
  sanitized: true
  sanitized_at: 2026-02-28T10:00:00Z
  sanitized_by: user
---

# Weather Skill

Get weather information without API keys.

## Tools

### wttr_current
**Description:** Get quick one-line weather for a location

**Command:**
```bash
curl -s "wttr.in/{{location}}?format=3"
```

**Parameters:**
- `location` (string): City name, airport code, or coordinates

**Timeout:** 10

**Examples:**
```bash
# London weather
wttr_current(location="London")

# New York weather
wttr_current(location="New+York")
```

---

### wttr_forecast
**Description:** Get full terminal forecast

**Command:**
```bash
curl -s "wttr.in/{{location}}?T"
```

**Parameters:**
- `location` (string): City name or coordinates

**Timeout:** 15

---

### open_meteo_current
**Description:** Get current weather using coordinates (JSON output)

**Command:**
```bash
curl -s "https://api.open-meteo.com/v1/forecast?latitude={{latitude}}&longitude={{longitude}}&current_weather=true"
```

**Parameters:**
- `latitude` (number): Latitude coordinate
- `longitude` (number): Longitude coordinate

**Timeout:** 10

**Validation:**
```yaml
allowed_commands:
  - curl
denied_patterns:
  - "rm -rf"
  - "sudo"
max_output_size: 102400
```
