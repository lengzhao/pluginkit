# pluginkit

启动期插件装配库：插件作者只注册类型，使用方用实例图和 root id 构造根插件；插件依赖既可以引用已有实例，也可以直接内联私有插件。

```
github.com/lengzhao/pluginkit          插件类型：Register / Lookup / Spec
github.com/lengzhao/pluginkit/config   配置识别：PluginUse / Parse
github.com/lengzhao/pluginkit/build    实例化：Build / BuildInto / GetByID
```

根包不读配置、不构造实例。`config` 不查注册表、不调用 `New`。`build` 不解释 `agent` / `workflow` / `etl` 等业务字段。

## 安装

```bash
go get github.com/lengzhao/pluginkit
```

## 插件作者

`init()` 只登记 `kind` 和构造函数：

```go
func init() {
    pluginkit.Register("openai", New)
}

func New(cfg Config) (*OpenAI, error) {
    return &OpenAI{model: cfg.Model}, nil
}
```

构造函数只支持：

```go
func New() (T, error)
func New(cfg Config) (T, error)
func New(cfg Config, deps Deps) (T, error)
```

## 使用方

默认用 root id 构造一张插件实例图：

```go
workflow, _, err := build.Build[Workflow](ctx, cfg, "workflow")
```

```yaml
workflow:
  use: sequential-workflow
  deps:
    steps:
      - use: http-step
      - use: save-step
        deps:
          store:
            use: sqlite-store
```

如果 `workflow.deps.steps` 里引用了不实现 `Step` 的实例，`Build[Workflow]` 会在 `deps` 阶段失败。

Agent 也可以建模成 root plugin：

```yaml
agent:
  use: agent
  deps:
    llm:
      use: openai
      config:
        model: gpt-5.5
    tools:
      - use: read-file
      - use: shell
```

内联实例未写 `id` 时按路径生成，例如 `workflow.steps[0]`。需要复用的实例放到顶层并用 id 引用。插件自己的 `config` 按 JSON 解到构造函数参数，未知字段会失败。

`BuildInto` 仍可用于直接填充 target struct，但 workflow、agent、ETL 这类编排应优先建模为 root plugin，把引用和内联子插件放进插件 `Deps`。

## 示例

```bash
cd examples/agent && go run .
cd examples/agent && go run . config.flat.yaml

cd examples/workflow && go run .
cd examples/workflow && go run . config.flat.yaml
```
