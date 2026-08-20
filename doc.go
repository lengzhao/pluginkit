// Package pluginkit 只登记插件类型：kind 与构造函数。
//
// init() 里应只调用 Register，不读取配置、不构造实例。
// 配置识别见 [github.com/lengzhao/pluginkit/config]，
// 启动期实例化见 [github.com/lengzhao/pluginkit/build]。
package pluginkit
