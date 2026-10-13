// @vitest-environment jsdom
// Synthetic UI contract; real restart/SQLite coverage is a separate Go test.
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { BatchHistoryPage, BatchResult } from "../lib/governance";
const { bridge, context } = vi.hoisted(() => ({
  bridge: { historyAvailable:vi.fn(), history:vi.fn(), result:vi.fn(), commit:vi.fn(), preview:vi.fn() },
  context: {} as Record<string,unknown>,
}));
vi.mock("../api/governance",()=>({governanceApi:bridge}));
vi.mock("../state/ProjectContext",()=>({useProject:()=>context}));
import GovernanceBatchHistory from "./GovernanceBatchHistory";
const id="artificial-batch-000001";
function history(): BatchHistoryPage { return {project_scope:"project-a",items:[{batch_id:id,status:"COMPLETE",requested_count:2,recorded_count:2,created_at:"2026-10-13T00:00:00Z"}],total_count:1,page:1,page_size:10}; }
function receipt(): BatchResult { return {batch_id:id,project_scope:"project-a",status:"COMPLETE",requested_count:2,counts:{total:2,saved:1,unchanged:0,conflict:1,blocked:0,failed:0},items:[{group_id:"g1",status:"SAVED",reason_code:"RECORDED_ONLY",decision_revision:1},{group_id:"g2",status:"CONFLICT",reason_code:"SUBJECT_CHANGED",decision_revision:0}]}; }
beforeEach(()=>{
  Object.assign(context,{project:{path:"/synthetic/project.db",project_id:"managed-project-a"},isReadWrite:false,capabilities:{can_edit_reviews:false,recovery_lock_active:true}});
  bridge.historyAvailable.mockReset().mockReturnValue(true);
  bridge.history.mockReset().mockResolvedValue(history());
  bridge.result.mockReset().mockResolvedValue(receipt());
  bridge.commit.mockReset();bridge.preview.mockReset();
});
afterEach(cleanup);
async function open() {fireEvent.click(screen.getByRole("button",{name:"查看已提交批次"})); await screen.findByRole("button",{name:"核对本页批次 1"});}
it("discovers durable receipts after remount without client keys even under readonly/recovery lock",async()=>{
  const first=render(<GovernanceBatchHistory expectedScope="project-a" refreshSignal=""/>);await open();first.unmount();
  render(<GovernanceBatchHistory expectedScope="project-a" refreshSignal=""/>);await open();
  fireEvent.click(screen.getByRole("button",{name:"核对本页批次 1"}));
  const result=await screen.findByRole("region",{name:"历史批次回执"});
  expect(within(result).getByText(/已记录 1 · 未变化 0 · 冲突 1/)).toBeVisible();
  expect(bridge.history).toHaveBeenCalledWith(1);expect(bridge.result).toHaveBeenCalledWith(id);
  expect(bridge.commit).not.toHaveBeenCalled();expect(bridge.preview).not.toHaveBeenCalled();
  expect(context.capabilities).toMatchObject({recovery_lock_active:true,can_edit_reviews:false});
});
it("keeps incomplete accounting explicit and never retries or completes it",async()=>{
  const page=history();page.items[0]={...page.items[0],status:"IN_PROGRESS",recorded_count:1};bridge.history.mockResolvedValue(page);
  const value=receipt();value.status="IN_PROGRESS";value.counts={total:2,saved:1,unchanged:0,conflict:0,blocked:0,failed:1};value.items[1]={group_id:"g2",status:"FAILED",reason_code:"NOT_COMMITTED",decision_revision:0};bridge.result.mockResolvedValue(value);
  render(<GovernanceBatchHistory expectedScope="project-a" refreshSignal=""/>);await open();fireEvent.click(screen.getByRole("button",{name:"核对本页批次 1"}));
  await screen.findByText("历史批次尚未结束");expect(screen.getByText(/不能把尚未持久化的项当成功/)).toBeVisible();
  expect(bridge.commit).not.toHaveBeenCalled();expect(screen.queryByRole("button",{name:/重试相同请求|保存决定/})).toBeNull();
});
it("rejects another project or batch receipt without leaking raw errors",async()=>{
  bridge.result.mockResolvedValue({...receipt(),project_scope:"project-b"});
  render(<GovernanceBatchHistory expectedScope="project-a" refreshSignal=""/>);await open();fireEvent.click(screen.getByRole("button",{name:"核对本页批次 1"}));
  await screen.findByText(/尚未取得可信的历史回执/);expect(screen.queryByRole("region",{name:"历史批次回执"})).toBeNull();
  bridge.result.mockRejectedValue(new Error("/private-artificial-anchor/file-name token-value"));
  fireEvent.click(screen.getByRole("button",{name:"核对本页批次 1"}));await waitFor(()=>expect(bridge.result).toHaveBeenCalledTimes(2));
  await waitFor(()=>expect(screen.getByRole("button",{name:"核对本页批次 1"})).not.toBeDisabled());
  expect(document.body.textContent).not.toContain("private-artificial-anchor");expect(bridge.commit).not.toHaveBeenCalled();
});
it("rejects false complete summaries and keeps older builds read-only",async()=>{
  const page=history();page.items[0].recorded_count=1;bridge.history.mockResolvedValue(page);
  const view=render(<GovernanceBatchHistory expectedScope="project-a" refreshSignal=""/>);fireEvent.click(screen.getByRole("button",{name:"查看已提交批次"}));
  await screen.findByText(/无法核对批次历史/);expect(screen.queryByRole("button",{name:"核对本页批次 1"})).toBeNull();
  bridge.historyAvailable.mockReturnValue(false);view.rerender(<GovernanceBatchHistory expectedScope="project-a" refreshSignal=""/>);
  expect(screen.getByText(/当前版本未提供批次发现接口/)).toBeVisible();expect(bridge.commit).not.toHaveBeenCalled();
});
it("ignores a late history response after switching project",async()=>{
  let resolve!:(value:BatchHistoryPage)=>void;bridge.history.mockReturnValueOnce(new Promise<BatchHistoryPage>(yes=>{resolve=yes;}));
  const view=render(<GovernanceBatchHistory expectedScope="project-a" refreshSignal=""/>);fireEvent.click(screen.getByRole("button",{name:"查看已提交批次"}));
  await waitFor(()=>expect(bridge.history).toHaveBeenCalledTimes(1));
  context.project={path:"/synthetic/other.db",project_id:"managed-project-b"};bridge.history.mockResolvedValue({project_scope:"project-b",items:[],total_count:0,page:1,page_size:10});
  view.rerender(<GovernanceBatchHistory expectedScope="project-b" refreshSignal=""/>);await screen.findByText(/当前项目没有此页批次记录/);
  await act(async()=>resolve(history()));expect(screen.queryByRole("button",{name:"核对本页批次 1"})).toBeNull();
});
it("pages bounded history and refreshes on an active batch outcome without resubmission",async()=>{
  const page=history();page.total_count=11;page.items=Array.from({length:10},(_,i)=>({...page.items[0],batch_id:id+i}));
  bridge.history.mockImplementation(async(n:number)=>n===1?page:{...page,page:2,items:[{...page.items[0],batch_id:id+"10"}]});
  const view=render(<GovernanceBatchHistory expectedScope="project-a" refreshSignal=""/>);await open();fireEvent.click(screen.getByRole("button",{name:"历史下一页"}));
  await screen.findByText("历史共 11 个批次；当前第 2 页。");expect(screen.getByRole("button",{name:"历史下一页"})).toBeDisabled();
  bridge.history.mockResolvedValue(history());view.rerender(<GovernanceBatchHistory expectedScope="project-a" refreshSignal="another-batch:COMPLETE:1"/>);
  await screen.findByText("历史共 1 个批次；当前第 1 页。");expect(bridge.commit).not.toHaveBeenCalled();
});

it("refuses a history list from a different scope before requesting any receipt",async()=>{
  bridge.history.mockResolvedValue({...history(),project_scope:"project-b"});
  render(<GovernanceBatchHistory expectedScope="project-a" refreshSignal=""/>);fireEvent.click(screen.getByRole("button",{name:"查看已提交批次"}));
  await screen.findByText(/无法核对批次历史/);expect(bridge.result).not.toHaveBeenCalled();expect(bridge.commit).not.toHaveBeenCalled();
});

it("does not display late receipt data after project replacement",async()=>{
  let resolve!:(value:BatchResult)=>void;bridge.result.mockReturnValueOnce(new Promise<BatchResult>(yes=>{resolve=yes;}));
  const view=render(<GovernanceBatchHistory expectedScope="project-a" refreshSignal=""/>);await open();fireEvent.click(screen.getByRole("button",{name:"核对本页批次 1"}));
  await waitFor(()=>expect(bridge.result).toHaveBeenCalledTimes(1));
  context.project={path:"/synthetic/project.db",project_id:"replacement-project"};
  bridge.history.mockResolvedValue({project_scope:"replacement-scope",items:[],total_count:0,page:1,page_size:10});
  view.rerender(<GovernanceBatchHistory expectedScope="replacement-scope" refreshSignal=""/>);await screen.findByText(/当前项目没有此页批次记录/);
  await act(async()=>resolve(receipt()));expect(screen.queryByRole("region",{name:"历史批次回执"})).toBeNull();
});
