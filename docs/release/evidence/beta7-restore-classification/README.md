# beta.7 分类证据专用载体

从仓库根执行：

```sh
python3 docs/release/evidence/beta7-restore-classification/run.py
```

要求 Python3、Git 和可用的 Go1.26.9 工具链。脚本仅导出固定 RC 到新的私有临时目录，复制 `.go.txt` 为该导出内的新测试，创建临时人工文件和数据库；不调用正式 App、不接触历史项目或 NAS。测试载体不进入产品源码/正式构建。

脚本输出脱敏 JSON，完整 stdout/stderr 和定位信息留在本机私有目录。`result.json`、`independent-result.json` 是两次最终载体执行的脱敏汇总。源码测试证明分类层，正式 GUI/文件与状态实测另见验收报告；不能声称取得签名 App 的原始运行字段。
