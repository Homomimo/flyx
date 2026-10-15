import type { InboundApiItem } from "@/api/types";

import { useEffect, useState } from "react";
import toast from "react-hot-toast";

import { importInbounds, updateExternalInbound } from "@/api";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Checkbox } from "@/shadcn-bridge/heroui/checkbox";
import { Input, Textarea } from "@/shadcn-bridge/heroui/input";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@/shadcn-bridge/heroui/modal";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";

const modalBase =
  "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden";

/** Label shown where a panel-managed inbound shows its node or kernel. */
export const EXTERNAL_LABEL = "导入的节点";

export function ImportModal({
  isOpen,
  onClose,
  onImported,
}: {
  isOpen: boolean;
  onClose: () => void;
  onImported: () => void;
}) {
  const [links, setLinks] = useState("");
  const [relayOnly, setRelayOnly] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setLinks("");
      setRelayOnly(false);
      setErrors([]);
    }
  }, [isOpen]);

  const submit = async () => {
    setSaving(true);
    try {
      const res = await importInbounds(links, relayOnly);
      const failed = res.data?.errors ?? [];

      setErrors(failed);
      if (res.code !== 0) {
        if (failed.length === 0) toast.error(res.msg || "导入失败");

        return;
      }
      const created = res.data?.created.length ?? 0;

      onImported();
      if (failed.length === 0) {
        toast.success(`已导入 ${created} 个节点`);
        onClose();
      } else {
        toast(`已导入 ${created} 个，${failed.length} 个失败`, { icon: "⚠️" });
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      backdrop="blur"
      classNames={{ base: modalBase }}
      isOpen={isOpen}
      placement="center"
      scrollBehavior="inside"
      size="lg"
      onOpenChange={(open) => !open && onClose()}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <h2 className="text-lg font-bold">导入节点</h2>
          <p className="text-small font-normal text-default-500">
            导入别处搭好的节点，用来对接中转和加入订阅。面板不会部署或修改这些节点。
          </p>
        </ModalHeader>
        <ModalBody>
          <Textarea
            description="每行一个链接，目前支持 ss://（不带插件）"
            label="分享链接"
            minRows={5}
            placeholder="ss://..."
            value={links}
            variant="bordered"
            onChange={(e) => setLinks(e.target.value)}
          />
          <Checkbox isSelected={relayOnly} onValueChange={setRelayOnly}>
            <span className="text-sm">订阅里只保留中转线路，不含直连</span>
          </Checkbox>
          {errors.length > 0 && (
            <ul className="space-y-1 rounded-xl bg-danger-50 p-3 text-xs text-danger">
              {errors.map((e) => (
                <li key={e} className="break-all">
                  {e}
                </li>
              ))}
            </ul>
          )}
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={onClose}>
            取消
          </Button>
          <Button
            color="primary"
            isDisabled={!links.trim()}
            isLoading={saving}
            onPress={submit}
          >
            导入
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}

interface EditForm {
  name: string;
  host: string;
  port: string;
  method: string;
  password: string;
  remark: string;
  relayOnly: boolean;
}

export function ExternalEditModal({
  item,
  ssMethods,
  onClose,
  onSaved,
}: {
  item: InboundApiItem | null;
  ssMethods: string[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [form, setForm] = useState<EditForm | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setForm(
      item
        ? {
            name: item.name,
            host: item.listen,
            port: String(item.port),
            method: item.settings.method ?? "",
            password: item.settings.password ?? "",
            remark: item.remark,
            relayOnly: item.relayOnly,
          }
        : null,
    );
  }, [item]);

  const set = (patch: Partial<EditForm>) =>
    setForm((p) => (p ? { ...p, ...patch } : p));

  const submit = async () => {
    if (!item || !form) return;
    setSaving(true);
    try {
      const res = await updateExternalInbound({
        id: item.id,
        name: form.name,
        host: form.host.trim(),
        port: Number(form.port),
        method: form.method,
        password: form.password,
        remark: form.remark,
        relayOnly: form.relayOnly,
      });

      if (res.code !== 0) {
        toast.error(res.msg || "保存失败");

        return;
      }
      if (res.msg && res.msg !== "操作成功") {
        toast(res.msg, { icon: "⚠️" });
      } else {
        toast.success("已保存");
      }
      onSaved();
      onClose();
    } finally {
      setSaving(false);
    }
  };

  const methods =
    form?.method && !ssMethods.includes(form.method)
      ? [...ssMethods, form.method]
      : ssMethods;

  return (
    <Modal
      backdrop="blur"
      classNames={{ base: modalBase }}
      isOpen={Boolean(item)}
      placement="center"
      scrollBehavior="inside"
      size="lg"
      onOpenChange={(open) => !open && onClose()}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <h2 className="text-lg font-bold">编辑导入的节点</h2>
          <p className="text-small font-normal text-default-500">
            只修改面板里的记录；原服务器上的配置变了才需要在这里同步
          </p>
        </ModalHeader>
        <ModalBody>
          {form && (
            <div className="space-y-3">
              <Input
                label="名称"
                value={form.name}
                variant="bordered"
                onChange={(e) => set({ name: e.target.value })}
              />
              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-2">
                  <Input
                    label="地址"
                    value={form.host}
                    variant="bordered"
                    onChange={(e) => set({ host: e.target.value })}
                  />
                </div>
                <Input
                  label="端口"
                  type="number"
                  value={form.port}
                  variant="bordered"
                  onChange={(e) => set({ port: e.target.value })}
                />
              </div>
              <Select
                label="加密方式"
                selectedKeys={form.method ? [form.method] : []}
                onSelectionChange={(keys) =>
                  set({ method: String(Array.from(keys)[0] ?? "") })
                }
              >
                {methods.map((m) => (
                  <SelectItem key={m} textValue={m}>
                    {m}
                  </SelectItem>
                ))}
              </Select>
              <Input
                label="密码"
                value={form.password}
                variant="bordered"
                onChange={(e) => set({ password: e.target.value })}
              />
              <Input
                label="备注"
                value={form.remark}
                variant="bordered"
                onChange={(e) => set({ remark: e.target.value })}
              />
              <Checkbox
                isSelected={form.relayOnly}
                onValueChange={(v) => set({ relayOnly: v })}
              >
                <span className="text-sm">订阅里只保留中转线路，不含直连</span>
              </Checkbox>
            </div>
          )}
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={onClose}>
            取消
          </Button>
          <Button color="primary" isLoading={saving} onPress={submit}>
            保存
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
