'use strict';
const element = (id) => document.getElementById(id);
const sourceRoute = '/gov/source-direct-purge';
const quarantineRoute = '/gov/quarantine-purge';
let choices = [];
let selected = [];

async function request(route, code, values) {
  const token = element('token').value.trim();
  if (!token) throw new Error('请输入私有访问令牌');
  const response = await fetch(route, {
    method: 'POST',
    credentials: 'omit',
    redirect: 'error',
    cache: 'no-store',
    headers: {'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token},
    body: JSON.stringify({op: code, ...values})
  });
  const result = await response.json();
  if (!response.ok) {
    const labels = {
      RECOVERY_REQUIRED: '需要恢复核验。已锁定进一步执行。',
      BUSY: '执行层正忙，请稍后查询回执。',
      REFUSED: '请求未通过当前状态、身份或保护核验。',
      govweb_unauthenticated: '访问令牌无效。',
      WORKER_RESULT_UNAVAILABLE: '执行层未返回结果。请先查询回执。'
    };
    const error = new Error(labels[result.reason] || '请求失败');
    error.plan = result.plan;
    throw error;
  }
  return result;
}

function button(label, run) {
  const control = document.createElement('button');
  control.type = 'button';
  control.textContent = label;
  control.addEventListener('click', run);
  return control;
}

function textCell(row, text) {
  const cell = row.insertCell();
  cell.textContent = text;
  return cell;
}

function sourceRows() {
  const body = element('files');
  body.replaceChildren();
  selected = [];
  for (const file of choices) {
    const row = body.insertRow();
    const check = document.createElement('input');
    check.type = 'checkbox';
    check.setAttribute('aria-label', '选择 ' + file.path);
    row.insertCell().append(check);
    textCell(row, file.path);
    textCell(row, String(file.size));
    textCell(row, file.sha256.slice(0, 12));
    textCell(row, file.role);
    const keeper = document.createElement('select');
    keeper.setAttribute('aria-label', file.path + ' 的保留副本');
    keeper.append(new Option('请选择保留副本', ''));
    for (const other of choices) {
      if (other.storage_id === file.storage_id && other.sha256 === file.sha256 && other.file_id !== file.file_id) {
        keeper.append(new Option(other.path, other.file_id));
      }
    }
    row.insertCell().append(keeper);
    check.disabled = keeper.options.length === 1;
    selected.push({file, check, keeper});
  }
}

function addPlan(route, prefix, initial, label) {
  let plan = initial;
  const panel = document.createElement('article');
  const title = document.createElement('h3');
  title.textContent = label;
  const details = document.createElement('p');
  const status = document.createElement('p');
  status.setAttribute('role', 'status');
  const controls = document.createElement('div');
  const buttons = [];
  let busy = false;
  const states = {
    DRAFT: '待审批', APPROVED: '已审批', DRY_RUN: 'Dry Run 通过',
    EXECUTING: '执行中或待恢复核验', PURGED: '已永久删除',
    ROLLED_BACK: '已回滚', CANCELLED: '已取消'
  };
  function update(message) {
    for (const action of buttons) {
      action.control.disabled = busy || (action.states && !action.states.includes(plan.state));
    }
    status.textContent = (states[plan.state] || '待确认') + (message ? '。' + message : '');
  }
  if (plan.keeper_path) {
    details.textContent = '全量哈希相同，已核验两个独立普通文件。保留 ' + plan.keeper_path +
      '。所选文件逻辑大小 ' + plan.size + ' 字节，实际释放空间取决于文件系统和快照。目录角色 ' + plan.role + '。';
  } else {
    details.textContent = '此计划仅清理到期且无 HOLD 的隔离文件。';
  }
  function action(suffix, label, statesAllowed) {
    const control = button(label, async () => {
      const values = {plan_id: plan.id, digest: plan.digest};
      if (suffix === 'EXECUTE') {
        const phrase = (prefix === 'SOURCE_DIRECT_PURGE_' ? '永久删除 ' : '永久清理 ') + plan.id;
        const entered = window.prompt('输入整句确认不可逆删除\n' + phrase);
        if (entered !== phrase) return;
        values.confirmation = entered;
      }
      busy = true;
      update('');
      let message = '';
      try {
        plan = await request(route, prefix + suffix, values);
      } catch (error) {
        if (error.plan && error.plan.id === plan.id) plan = error.plan;
        message = error.message;
      } finally {
        busy = false;
        update(message);
      }
    });
    controls.append(control, ' ');
    buttons.push({control, states: statesAllowed});
  }
  action('APPROVE', '单项审批', ['DRAFT']);
  action('DRY_RUN', 'Dry Run', ['APPROVED']);
  action('EXECUTE', '确认并永久删除', ['DRY_RUN']);
  action('CANCEL', '取消此计划', ['DRAFT', 'APPROVED', 'DRY_RUN']);
  action('RECEIPT', '查询回执', null);
  action('RECOVER', '核验并尝试恢复', ['EXECUTING']);
  panel.append(title, details, controls, status);
  element('plans').append(panel);
  update('');
}

function pageAction(id, run) {
  element(id).addEventListener('click', async (event) => {
    const control = event.currentTarget;
    control.disabled = true;
    element('status').textContent = '';
    try {
      await run();
    } catch (error) {
      element('status').textContent = error.message;
    } finally {
      control.disabled = false;
    }
  });
}

pageAction('load', async () => {
  const result = await request(sourceRoute, 'SOURCE_DIRECT_PURGE_LIST', {});
  choices = result.choices || [];
  sourceRows();
  element('status').textContent = '已加载 ' + choices.length + ' 项扫描候选';
});

pageAction('quarantine', async () => {
  const item = element('item').value.trim();
  if (!item) throw new Error('请输入隔离区项目 ID');
  const plan = await request(quarantineRoute, 'QUARANTINE_PURGE_PLAN', {item_id: item});
  addPlan(quarantineRoute, 'QUARANTINE_PURGE_', plan, '隔离区项目 ' + item);
});

pageAction('plan', async () => {
  const rows = selected.filter((row) => row.check.checked);
  if (rows.length === 0) throw new Error('请至少选择一个文件');
  const targetIds = new Set(rows.map(({file}) => file.file_id));
  for (const {file, check, keeper} of rows) {
    try {
      if (!keeper.value || targetIds.has(keeper.value)) throw new Error('请选择未被选中删除的保留副本');
      const plan = await request(sourceRoute, 'SOURCE_DIRECT_PURGE_PLAN', {
        storage_id: file.storage_id, target_id: file.file_id, keeper_id: keeper.value
      });
      addPlan(sourceRoute, 'SOURCE_DIRECT_PURGE_', plan, file.path);
      check.checked = false;
    } catch (error) {
      const result = document.createElement('p');
      result.textContent = file.path + '。' + error.message;
      element('plans').append(result);
    }
  }
});
