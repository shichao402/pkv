<!-- 本文件由 `dec pull` 从 .dec/cache/cli/ 渲染生成，请勿直接编辑。
     修改流程：编辑 .dec/cache/cli/... → dec push → dec pull 验证 -->

---
name: cli-release-workflow
description: >
  CLI 项目发布执行 skill。用于在收到“发布”指令时，由 agent 自主判断当前仓库状态，并完成版本提升、提交、推送，让 CI 继续正式发布。
---

# CLI 发布执行 Skill
