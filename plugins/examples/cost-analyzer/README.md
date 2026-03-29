# Cost Analyzer Plugin

A Kite plugin that estimates Kubernetes resource costs per namespace based on CPU and memory requests.

## Features

- **Cost Dashboard**: Visual breakdown of costs per namespace with charts
- **AI Tool**: Ask "How much does the production namespace cost?" via Kite AI
- **Configurable Pricing**: Set custom CPU/memory hourly rates in settings
- **REST API**: Programmatic access to cost data

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/costs?namespace=X` | Cost breakdown (optional namespace filter) |
| GET | `/costs/summary` | Aggregated cost summary across all namespaces |
| PUT | `/settings` | Update pricing configuration |

## AI Tools

| Tool | Description |
|------|-------------|
| `get_namespace_cost` | Calculate estimated cost for a specific namespace |

## Development

```bash
# Build plugin
make build

# Run tests
make test
```

## Configuration

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `cpuPricePerHour` | number | 0.05 | Cost per CPU core per hour (USD) |
| `memoryPricePerGBHour` | number | 0.01 | Cost per GB memory per hour (USD) |
