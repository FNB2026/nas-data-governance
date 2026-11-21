(async (page, token, item, phase) => {
  const label = name => page.getByLabel(name, {exact: true});
  const button = (scope, name) => scope.getByRole('button', {name, exact: true});
  const status = (scope, message) => scope.getByRole('status').filter({hasText: message}).waitFor({timeout: 10_000});
  const press = (scope, name) => button(scope, name).click({timeout: 5000});
  const plan = title => page.getByRole('article').filter({has: page.getByRole('heading', {name: title, exact: true})});
  const sourceFile = (group, role) => '/ndg-volume/temp/' + group + '-' + role + '.pdf';
  const reference = async panel => {
    const value = await panel.getByText(/^计划 ID /).textContent({timeout: 5000});
    const fields = /^计划 ID ([a-f0-9]+) · 审批摘要 ([a-f0-9]+)$/.exec(value);
    if (!fields) throw new Error('Browser plan reference missing');
    return {id: fields[1], digest: fields[2]};
  };
  const approveAndDryRun = async panel => {
    if (await button(panel, '确认并永久删除').isEnabled()) throw new Error('Execution enabled before approval');
    await press(panel, '单项审批');
    await status(panel, '已审批');
    await press(panel, 'Dry Run');
    await status(panel, 'Dry Run 通过');
  };

  if (phase === 'prepare-source') {
    const deadline = Date.now() + 45_000;
    while (true) {
      try {
        await page.goto('https://10.254.140.2:8443/gov/purge-console', {timeout: 3000});
        await page.getByRole('heading', {name: 'NDG 永久删除'}).waitFor({timeout: 1000});
        break;
      } catch (error) {
        if (Date.now() >= deadline) throw error;
        await new Promise(resolve => setTimeout(resolve, 500));
      }
    }
    await press(page, '加载扫描候选');
    await status(page, '请输入私有访问令牌');
    await label('私有访问令牌').fill(token);
    await press(page, '加载扫描候选');
    await status(page, '已加载 6 项扫描候选');
    for (const group of ['one', 'two']) {
      const file = sourceFile(group, 'remove');
      await label('选择 ' + file).check();
      await label(file + ' 的保留副本').selectOption({label: sourceFile(group, 'keeper')});
    }
    await press(page, '为所选文件分别创建计划');
    const first = plan(sourceFile('one', 'remove'));
    const second = plan(sourceFile('two', 'remove'));
    await status(first, '待审批');
    await status(second, '待审批');
    const firstRef = await reference(first);
    const secondRef = await reference(second);
    if (firstRef.id === secondRef.id) throw new Error('Batch selection reused a plan');
    await approveAndDryRun(first);
    await status(second, '待审批');
    await press(second, '取消此计划');
    await status(second, '已取消');
    await press(first, '确认并永久删除');
    return;
  }

  const first = plan(sourceFile('one', 'remove'));
  const second = plan(sourceFile('two', 'remove'));
  if (phase === 'confirm-source') {
    await status(first, 'Dry Run 通过');
    await press(first, '确认并永久删除');
    return;
  }
  await status(first, '已永久删除');
  if (phase === 'prepare-quarantine') {
    await label('隔离区项目 ID').fill(item);
    await press(page, '为此项目创建计划');
    const quarantined = plan('隔离区项目 ' + item);
    await status(quarantined, '待审批');
    await approveAndDryRun(quarantined);
    await press(quarantined, '确认并永久删除');
    return;
  }
  if (phase !== 'verify-reload') throw new Error('Unknown browser acceptance phase');
  const quarantined = plan('隔离区项目 ' + item);
  await status(quarantined, '已永久删除');
  const firstRef = await reference(first);
  const secondRef = await reference(second);
  const quarantineRef = await reference(quarantined);
  await page.reload();
  await label('私有访问令牌').fill(token);
  await page.getByText('查询已保存的计划', {exact: true}).click();
  for (const [mode, ref, title, state] of [
    ['source', firstRef, 'temp/one-remove.pdf', '已永久删除'],
    ['source', secondRef, 'temp/two-remove.pdf', '已取消'],
    ['quarantine', quarantineRef, '隔离区项目 ' + item, '已永久删除'],
  ]) {
    await label('操作').selectOption(mode);
    await label('计划 ID').fill(ref.id);
    await label('审批摘要').fill(ref.digest);
    await button(page.locator('details'), '查询回执').click();
    await status(plan(title), state);
  }
  return 'NDG browser verification complete';
})
