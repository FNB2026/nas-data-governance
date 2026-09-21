// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import DisabledNotice from "./DisabledNotice";

afterEach(cleanup);

describe("DisabledNotice", () => {
  it("renders reason with status role", () => {
    render(<DisabledNotice reason="NAS 连接中断" />);
    const notice = screen.getByRole("status");
    expect(notice).toHaveTextContent("NAS 连接中断");
  });

  it("renders hint and fires action", () => {
    const onAction = vi.fn();
    render(
      <DisabledNotice
        reason="只读模式，无法执行写操作"
        hint="当前为只读模式"
        actionLabel="重新检查"
        onAction={onAction}
      />,
    );
    expect(screen.getByText("当前为只读模式")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "重新检查" }));
    expect(onAction).toHaveBeenCalledTimes(1);
  });
});