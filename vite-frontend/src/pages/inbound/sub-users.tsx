import type { SubUserItem } from "@/api";
import type { InboundApiItem } from "@/api/types";

import { useCallback, useEffect, useState } from "react";
import toast from "react-hot-toast";
import { Copy } from "lucide-react";

import { deleteSubUser, getSubUsers, resetSubUser, saveSubUser } from "@/api";
import { copyText, panelOrigin } from "@/pages/inbound/share";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Card, CardBody } from "@/shadcn-bridge/heroui/card";
import { Checkbox } from "@/shadcn-bridge/heroui/checkbox";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import { Input } from "@/shadcn-bridge/heroui/input";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@/shadcn-bridge/heroui/modal";
import { Switch } from "@/shadcn-bridge/heroui/switch";

const modalBase =
  "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden";

/** Inbounds whose credentials can't be per user (mirrors the backend). */
const MULTI_USER_PROTOCOLS = new Set([
  "vless",
  "vmess",
  "trojan",
  "hysteria2",
  "tuic",
  "anytls",
  "naive",
]);

function sharedCredentials(i: InboundApiItem): boolean {
  return i.external || !MULTI_USER_PROTOCOLS.has(i.protocol);
}

function toDateInput(ms: number): string {
  if (!ms) return "";
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, "0");

  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** The end of the chosen local day, so a user stays valid through it. */
function fromDateInput(value: string): number {
  if (!value) return 0;
  const [y, m, d] = value.split("-").map(Number);

  return new Date(y, m - 1, d, 23, 59, 59).getTime();
}

function expiryLabel(u: SubUserItem): string {
  if (!u.expireTime) return "永不过期";
  const days = Math.ceil((u.expireTime - Date.now()) / 86400000);

  if (days <= 0) return `${toDateInput(u.expireTime)} 已过期`;

  return `${toDateInput(u.expireTime)} 到期 · 剩 ${days} 天`;
}

interface Draft {
  id?: number;
  name: string;
  expire: string;
  inboundIds: number[];
  remark: string;
  enabled: boolean;
}

const emptyDraft = (): Draft => ({
  name: "",
  expire: "",
  inboundIds: [],
  remark: "",
  enabled: true,
});

/** People the admin shares inbounds with, each with an own link. */
export function SubUsersCard({ inbounds }: { inbounds: InboundApiItem[] }) {
  const [users, setUsers] = useState<SubUserItem[]>([]);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saving, setSaving] = useState(false);
  const [confirm, setConfirm] = useState<"reset" | "delete" | null>(null);
  const [toggling, setToggling] = useState<number | null>(null);

  const load = useCallback(async () => {
    const res = await getSubUsers();

    if (res.code === 0) setUsers(res.data ?? []);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const link = (u: SubUserItem) => `${panelOrigin()}${u.path}`;

  const showResult = (msg: string | undefined, ok: string) => {
    if (msg && msg !== "操作成功") toast(msg, { icon: "⚠️" });
    else toast.success(ok);
  };

  const save = async () => {
    if (!draft) return;
    setSaving(true);
    try {
      const res = await saveSubUser({
        id: draft.id,
        name: draft.name,
        inboundIds: draft.inboundIds,
        expireTime: fromDateInput(draft.expire),
        enabled: draft.enabled,
        remark: draft.remark,
      });

      if (res.code !== 0) {
        toast.error(res.msg || "保存失败");

        return;
      }
      showResult(res.msg, draft.id ? "已保存" : "已添加，复制链接发给对方即可");
      setDraft(null);
      await load();
    } finally {
      setSaving(false);
    }
  };

  const toggle = async (u: SubUserItem) => {
    setToggling(u.id);
    try {
      const res = await saveSubUser({
        id: u.id,
        name: u.name,
        inboundIds: u.inboundIds,
        expireTime: u.expireTime,
        enabled: !u.enabled,
        remark: u.remark,
      });

      if (res.code !== 0) toast.error(res.msg || "操作失败");
      else showResult(res.msg, u.enabled ? "已停用" : "已启用");
      await load();
    } finally {
      setToggling(null);
    }
  };

  const runConfirm = async () => {
    if (!draft?.id || !confirm) return;
    setSaving(true);
    try {
      const res =
        confirm === "reset"
          ? await resetSubUser(draft.id)
          : await deleteSubUser(draft.id);

      if (res.code !== 0) {
        toast.error(res.msg || "操作失败");

        return;
      }
      showResult(
        res.msg,
        confirm === "reset" ? "已重置，请把新链接发给对方" : "已删除",
      );
      setConfirm(null);
      setDraft(null);
      await load();
    } finally {
      setSaving(false);
    }
  };

  const selectable = inbounds.filter(
    (i) => i.enabled || draft?.inboundIds.includes(i.id),
  );
  const allSelected =
    selectable.length > 0 &&
    selectable.every((i) => draft?.inboundIds.includes(i.id));
  const sharedSelected = draft
    ? inbounds.filter(
        (i) => draft.inboundIds.includes(i.id) && sharedCredentials(i),
      )
    : [];

  return (
    <Card className="shadow-sm">
      <CardBody className="space-y-3">
        <div className="flex items-center justify-between gap-2">
          <div>
            <h3 className="text-base font-semibold">共享用户</h3>
            <p className="text-xs text-default-500">
              给朋友单独的订阅和凭据，到期或停用后自动断开
            </p>
          </div>
          <Button
            color="primary"
            isDisabled={inbounds.length === 0}
            size="sm"
            variant="flat"
            onPress={() => setDraft(emptyDraft())}
          >
            添加
          </Button>
        </div>

        {users.length === 0 ? (
          <p className="py-4 text-center text-xs text-default-500">
            还没有共享用户
          </p>
        ) : (
          <div className="space-y-2">
            {users.map((u) => (
              <div
                key={u.id}
                className="flex cursor-pointer items-center gap-3 rounded-xl border border-divider px-3 py-2.5 transition-colors hover:bg-default-100"
                role="button"
                tabIndex={0}
                onClick={() =>
                  setDraft({
                    id: u.id,
                    name: u.name,
                    expire: toDateInput(u.expireTime),
                    inboundIds: u.inboundIds,
                    remark: u.remark,
                    enabled: u.enabled,
                  })
                }
                onKeyDown={(e) => {
                  if (e.target !== e.currentTarget) return;
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    (e.currentTarget as HTMLElement).click();
                  }
                }}
              >
                <span
                  className={`h-2 w-2 shrink-0 rounded-full ${
                    u.active ? "bg-success" : "bg-default-300"
                  }`}
                />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5">
                    <span className="truncate font-medium">{u.name}</span>
                    {!u.enabled ? (
                      <Chip size="sm" variant="flat">
                        已停用
                      </Chip>
                    ) : (
                      !u.active && (
                        <Chip color="danger" size="sm" variant="flat">
                          已过期
                        </Chip>
                      )
                    )}
                  </div>
                  <div className="truncate text-xs text-default-500">
                    {expiryLabel(u)} · {u.inboundIds.length} 个入站
                    {u.remark ? ` · ${u.remark}` : ""}
                  </div>
                </div>
                <div
                  className="flex shrink-0 items-center gap-2"
                  role="presentation"
                  onClick={(e) => e.stopPropagation()}
                >
                  <Button
                    isIconOnly
                    aria-label={`复制 ${u.name} 的订阅链接`}
                    size="sm"
                    variant="flat"
                    onPress={() => copyText(link(u), "订阅链接已复制")}
                  >
                    <Copy className="h-4 w-4" />
                  </Button>
                  <Switch
                    isDisabled={toggling === u.id}
                    isSelected={u.enabled}
                    size="sm"
                    onValueChange={() => toggle(u)}
                  />
                </div>
              </div>
            ))}
          </div>
        )}
      </CardBody>

      <Modal
        backdrop="blur"
        classNames={{ base: modalBase }}
        isOpen={Boolean(draft) && !confirm}
        placement="center"
        scrollBehavior="inside"
        onOpenChange={(open) => !open && setDraft(null)}
      >
        <ModalContent>
          {(onClose) =>
            draft && (
              <>
                <ModalHeader>
                  <h2 className="text-lg font-bold">
                    {draft.id ? "编辑共享用户" : "添加共享用户"}
                  </h2>
                </ModalHeader>
                <ModalBody className="space-y-3">
                  <Input
                    label="名称"
                    placeholder="例如 小明"
                    value={draft.name}
                    variant="bordered"
                    onChange={(e) =>
                      setDraft({ ...draft, name: e.target.value })
                    }
                  />
                  <div className="space-y-1">
                    <label
                      className="text-sm font-medium"
                      htmlFor="sub-user-expire"
                    >
                      到期日期
                    </label>
                    <div className="flex gap-2">
                      <input
                        className="h-10 min-w-0 flex-1 rounded-lg border border-divider bg-background px-2 text-sm"
                        id="sub-user-expire"
                        type="date"
                        value={draft.expire}
                        onChange={(e) =>
                          setDraft({ ...draft, expire: e.target.value })
                        }
                      />
                      {[30, 90, 365].map((d) => (
                        <Button
                          key={d}
                          className="min-w-0 px-2"
                          size="sm"
                          variant="flat"
                          onPress={() =>
                            setDraft({
                              ...draft,
                              expire: toDateInput(Date.now() + d * 86400000),
                            })
                          }
                        >
                          {d === 365 ? "1年" : `${d}天`}
                        </Button>
                      ))}
                    </div>
                    <p className="text-xs text-default-500">
                      {draft.expire
                        ? "到期当天 23:59 后断开"
                        : "留空表示永不过期"}
                    </p>
                  </div>

                  <div className="space-y-1.5">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium">
                        共享的入站（{draft.inboundIds.length}）
                      </span>
                      <Button
                        size="sm"
                        variant="light"
                        onPress={() =>
                          setDraft({
                            ...draft,
                            inboundIds: allSelected
                              ? []
                              : selectable.map((i) => i.id),
                          })
                        }
                      >
                        {allSelected ? "取消全选" : "全选"}
                      </Button>
                    </div>
                    <div className="max-h-56 space-y-1 overflow-y-auto rounded-xl border border-divider p-2">
                      {selectable.map((i) => (
                        <Checkbox
                          key={i.id}
                          className="w-full"
                          isSelected={draft.inboundIds.includes(i.id)}
                          onValueChange={(on) =>
                            setDraft({
                              ...draft,
                              inboundIds: on
                                ? [...draft.inboundIds, i.id]
                                : draft.inboundIds.filter((x) => x !== i.id),
                            })
                          }
                        >
                          <span className="text-sm">{i.name}</span>
                          {sharedCredentials(i) && (
                            <span className="ml-1 text-xs text-warning">
                              共用凭据
                            </span>
                          )}
                        </Checkbox>
                      ))}
                    </div>
                    {sharedSelected.length > 0 && (
                      <p className="text-xs text-warning">
                        其中 {sharedSelected.length} 个是导入的节点或
                        Shadowsocks /
                        Snell，没法单独分配凭据：对方拿到的是共用的密码，停用或到期后订阅失效，但已经导入的配置仍能连接。
                      </p>
                    )}
                  </div>

                  <Input
                    label="备注"
                    placeholder="可选"
                    value={draft.remark}
                    variant="bordered"
                    onChange={(e) =>
                      setDraft({ ...draft, remark: e.target.value })
                    }
                  />

                  {draft.id && (
                    <div className="flex flex-wrap gap-2 border-t border-divider pt-3">
                      <Button
                        size="sm"
                        variant="flat"
                        onPress={() => {
                          const u = users.find((x) => x.id === draft.id);

                          if (u) copyText(link(u), "订阅链接已复制");
                        }}
                      >
                        复制订阅链接
                      </Button>
                      <Button
                        color="warning"
                        size="sm"
                        variant="flat"
                        onPress={() => setConfirm("reset")}
                      >
                        重置链接和凭据
                      </Button>
                      <Button
                        color="danger"
                        size="sm"
                        variant="flat"
                        onPress={() => setConfirm("delete")}
                      >
                        删除
                      </Button>
                    </div>
                  )}
                </ModalBody>
                <ModalFooter>
                  <Button variant="light" onPress={onClose}>
                    取消
                  </Button>
                  <Button color="primary" isLoading={saving} onPress={save}>
                    保存
                  </Button>
                </ModalFooter>
              </>
            )
          }
        </ModalContent>
      </Modal>

      <Modal
        backdrop="blur"
        classNames={{ base: modalBase }}
        isOpen={Boolean(confirm)}
        placement="center"
        onOpenChange={(open) => !open && setConfirm(null)}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>
                <h2
                  className={`text-lg font-bold ${confirm === "delete" ? "text-danger" : ""}`}
                >
                  {confirm === "reset" ? "重置链接和凭据" : "删除共享用户"}
                </h2>
              </ModalHeader>
              <ModalBody>
                <p className="text-sm text-default-600">
                  {confirm === "reset"
                    ? "旧的订阅链接和对方已导入的配置会立即失效，需要把新链接发给对方重新导入。"
                    : "对方的订阅链接和凭据会立即失效，无法恢复。"}
                </p>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color={confirm === "delete" ? "danger" : "warning"}
                  isLoading={saving}
                  onPress={runConfirm}
                >
                  确认
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </Card>
  );
}
