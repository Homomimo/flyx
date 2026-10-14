import type {
  KernelApiItem,
  KernelCapabilitiesApiData,
  NodeApiItem,
} from "@/api/types";

import { useMemo, useState } from "react";
import toast from "react-hot-toast";

import {
  PRESETS,
  PROTOCOL_LABELS,
  SECURITY_LABELS,
  TRANSPORT_LABELS,
  type InboundForm,
  applyPreset,
  protocolCap,
  toPayload,
  visionAllowed,
} from "./presets";

import { createInbound, updateInbound } from "@/api";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Input, Textarea } from "@/shadcn-bridge/heroui/input";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@/shadcn-bridge/heroui/modal";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import { Switch } from "@/shadcn-bridge/heroui/switch";

interface Props {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  caps: KernelCapabilitiesApiData | null;
  nodes: NodeApiItem[];
  kernels: KernelApiItem[];
  initial: InboundForm;
  onSaved: () => void;
  onInstallKernel: (nodeId: number, kernelType: string) => void;
}

const XHTTP_MODES = ["auto", "packet-up", "stream-up", "stream-one"];

export function eligibleNodes(nodes: NodeApiItem[]): NodeApiItem[] {
  return nodes.filter(
    (n) => Number(n.isRemote ?? 0) !== 1 && n.forwardMode !== "nftables",
  );
}

export function InboundFormModal({
  isOpen,
  onOpenChange,
  caps,
  nodes,
  kernels,
  initial,
  onSaved,
  onInstallKernel,
}: Props) {
  const [form, setForm] = useState<InboundForm>(initial);
  const [initialKey, setInitialKey] = useState(initial);
  const [saving, setSaving] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const isEdit = Boolean(initial.id);

  // Reset when the parent opens the modal with a different item.
  if (initialKey !== initial) {
    setInitialKey(initial);
    setForm(initial);
    setShowAdvanced(false);
  }

  const set = <K extends keyof InboundForm>(key: K, value: InboundForm[K]) =>
    setForm((prev) => ({ ...prev, [key]: value }));

  const implementedKernels = useMemo(
    () => (caps?.kernels ?? []).filter((k) => k.implemented),
    [caps],
  );
  const kernelInfo = caps?.kernels.find((k) => k.type === form.kernelType);
  const installedKernel = kernels.find(
    (k) => String(k.nodeId) === form.nodeId && k.kernelType === form.kernelType,
  );
  const pCap = protocolCap(caps, form.kernelType, form.protocol);
  const transports = pCap?.transports ?? [];
  const securities =
    transports.find((t) => t.type === form.transportType)?.securities ?? [];

  // The Reality SNI is a borrowed public site; it must never leak into a TLS
  // SNI (a self-signed cert would be issued for it), and vice versa.
  const nextSni = (prevSecurity: string, prevSni: string, security: string) => {
    if (security === "reality") {
      return prevSecurity === "reality" && prevSni
        ? prevSni
        : (caps?.defaults.realitySni ?? "");
    }

    return prevSecurity === "reality" ? "" : prevSni;
  };

  const pickSecurity = (secs: string[], current: string) =>
    secs.includes(current) ? current : (secs[0] ?? "none");

  const changeProtocol = (protocol: string) => {
    const cap = protocolCap(caps, form.kernelType, protocol);
    const transport = cap?.transports.some((t) => t.type === form.transportType)
      ? form.transportType
      : (cap?.transports[0]?.type ?? "tcp");
    const secs =
      cap?.transports.find((t) => t.type === transport)?.securities ?? [];

    setForm((prev) => {
      const security = pickSecurity(secs, prev.securityType);

      return {
        ...prev,
        protocol,
        transportType: transport,
        securityType: security,
        sni: nextSni(prev.securityType, prev.sni, security),
      };
    });
  };

  const changeTransport = (transportType: string) => {
    const secs =
      transports.find((t) => t.type === transportType)?.securities ?? [];

    setForm((prev) => {
      const security = pickSecurity(secs, prev.securityType);

      return {
        ...prev,
        transportType,
        securityType: security,
        sni: nextSni(prev.securityType, prev.sni, security),
      };
    });
  };

  const changeSecurity = (securityType: string) => {
    setForm((prev) => ({
      ...prev,
      securityType,
      sni: nextSni(prev.securityType, prev.sni, securityType),
    }));
  };

  const validate = (): string | null => {
    if (!form.nodeId) return "请选择节点";
    if (!installedKernel) return "该节点还没有安装所选内核";
    if (!form.name.trim()) return "请输入名称";
    if (form.port.trim()) {
      const port = Number(form.port);

      if (!Number.isInteger(port) || port < 1 || port > 65535) {
        return "端口必须在 1-65535 之间";
      }
    }
    if (form.securityType === "tls" && !form.sni.trim()) {
      return "TLS 需要填写域名";
    }
    if (form.securityType === "tls" && form.certMode === "file") {
      if (!form.certFile.trim() || !form.keyFile.trim()) {
        return "请填写证书和私钥在节点上的路径";
      }
    }
    if (form.extra.trim()) {
      try {
        const parsed = JSON.parse(form.extra);

        if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
          return "高级配置必须是 JSON 对象";
        }
      } catch {
        return "高级配置不是合法的 JSON";
      }
    }

    return null;
  };

  const submit = async () => {
    const err = validate();

    if (err) {
      toast.error(err);

      return;
    }
    setSaving(true);
    try {
      const payload = toPayload(form);
      const res = isEdit
        ? await updateInbound(payload)
        : await createInbound(payload);

      if (res.code !== 0) {
        toast.error(res.msg || "保存失败");

        return;
      }
      if (res.msg && res.msg !== "操作成功") {
        toast(res.msg, { icon: "⚠️", duration: 6000 });
      } else {
        toast.success(isEdit ? "已保存并下发" : "已创建并下发");
      }
      onOpenChange(false);
      onSaved();
    } finally {
      setSaving(false);
    }
  };

  const nodeOptions = eligibleNodes(nodes);

  return (
    <Modal
      backdrop="blur"
      classNames={{
        base: "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden",
      }}
      isOpen={isOpen}
      placement="center"
      scrollBehavior="inside"
      size="2xl"
      onOpenChange={onOpenChange}
    >
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader className="flex flex-col gap-1">
              <h2 className="text-xl font-bold">
                {isEdit ? "编辑入站" : "新建入站"}
              </h2>
              <p className="text-small text-default-500">
                留空的 UUID、密码、密钥会自动生成，端口留空则随机分配
              </p>
            </ModalHeader>
            <ModalBody>
              <div className="space-y-4">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <Select
                    isDisabled={isEdit}
                    label="节点"
                    placeholder="选择落地节点"
                    selectedKeys={form.nodeId ? [form.nodeId] : []}
                    onSelectionChange={(keys) =>
                      set("nodeId", String(Array.from(keys)[0] ?? ""))
                    }
                  >
                    {nodeOptions.map((n) => (
                      <SelectItem
                        key={String(n.id)}
                        textValue={
                          n.status === 1 ? n.name : `${n.name}（离线）`
                        }
                      >
                        {n.status === 1 ? n.name : `${n.name}（离线）`}
                      </SelectItem>
                    ))}
                  </Select>
                  <Select
                    isDisabled={isEdit}
                    label="内核"
                    selectedKeys={[form.kernelType]}
                    onSelectionChange={(keys) =>
                      set("kernelType", String(Array.from(keys)[0] ?? "xray"))
                    }
                  >
                    {implementedKernels.map((k) => (
                      <SelectItem key={k.type} textValue={k.name}>
                        {k.name}
                      </SelectItem>
                    ))}
                  </Select>
                </div>

                {form.nodeId && !installedKernel && (
                  <div className="flex items-center justify-between gap-3 rounded-xl bg-warning-50 dark:bg-warning-500/10 px-3 py-2 text-sm">
                    <span className="text-warning-700 dark:text-warning-400">
                      该节点还没有安装 {kernelInfo?.name ?? form.kernelType}
                    </span>
                    <Button
                      color="warning"
                      size="sm"
                      variant="flat"
                      onPress={() =>
                        onInstallKernel(Number(form.nodeId), form.kernelType)
                      }
                    >
                      去安装
                    </Button>
                  </div>
                )}

                {!isEdit && (
                  <div>
                    <p className="text-sm font-medium mb-2">快速模板</p>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                      {PRESETS.filter((p) =>
                        protocolCap(caps, form.kernelType, p.protocol),
                      ).map((p) => {
                        const active =
                          form.protocol === p.protocol &&
                          form.transportType === p.transport &&
                          form.securityType === p.security;

                        return (
                          <button
                            key={p.key}
                            className={`text-left rounded-xl border px-3 py-2 transition-colors ${
                              active
                                ? "border-primary bg-primary-50 dark:bg-primary-500/10"
                                : "border-divider hover:bg-default-100"
                            }`}
                            type="button"
                            onClick={() =>
                              setForm((prev) => applyPreset(prev, p, caps))
                            }
                          >
                            <div className="text-sm font-medium">{p.label}</div>
                            <div className="text-xs text-default-500">
                              {p.description}
                            </div>
                          </button>
                        );
                      })}
                    </div>
                  </div>
                )}

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <Input
                    label="名称"
                    placeholder="例如：东京-Reality"
                    value={form.name}
                    variant="bordered"
                    onChange={(e) => set("name", e.target.value)}
                  />
                  <Input
                    label="端口"
                    placeholder="留空随机"
                    type="number"
                    value={form.port}
                    variant="bordered"
                    onChange={(e) => set("port", e.target.value)}
                  />
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                  <Select
                    label="协议"
                    selectedKeys={[form.protocol]}
                    onSelectionChange={(keys) =>
                      changeProtocol(String(Array.from(keys)[0] ?? "vless"))
                    }
                  >
                    {(kernelInfo?.protocols ?? []).map((p) => (
                      <SelectItem
                        key={p.protocol}
                        textValue={PROTOCOL_LABELS[p.protocol] ?? p.protocol}
                      >
                        {PROTOCOL_LABELS[p.protocol] ?? p.protocol}
                      </SelectItem>
                    ))}
                  </Select>
                  <Select
                    isDisabled={transports.length <= 1}
                    label="传输"
                    selectedKeys={[form.transportType]}
                    onSelectionChange={(keys) =>
                      changeTransport(String(Array.from(keys)[0] ?? "tcp"))
                    }
                  >
                    {transports.map((t) => (
                      <SelectItem
                        key={t.type}
                        textValue={TRANSPORT_LABELS[t.type] ?? t.type}
                      >
                        {TRANSPORT_LABELS[t.type] ?? t.type}
                      </SelectItem>
                    ))}
                  </Select>
                  <Select
                    isDisabled={securities.length <= 1}
                    label="安全"
                    selectedKeys={[form.securityType]}
                    onSelectionChange={(keys) =>
                      changeSecurity(String(Array.from(keys)[0] ?? "none"))
                    }
                  >
                    {securities.map((s) => (
                      <SelectItem key={s} textValue={SECURITY_LABELS[s] ?? s}>
                        {SECURITY_LABELS[s] ?? s}
                      </SelectItem>
                    ))}
                  </Select>
                </div>

                {(form.protocol === "vless" || form.protocol === "vmess") && (
                  <Input
                    label="UUID"
                    placeholder="留空自动生成"
                    value={form.uuid}
                    variant="bordered"
                    onChange={(e) => set("uuid", e.target.value)}
                  />
                )}
                {visionAllowed(form) && (
                  <div className="flex items-center gap-2">
                    <Switch
                      isSelected={form.flowVision}
                      size="sm"
                      onValueChange={(v) => set("flowVision", v)}
                    />
                    <span className="text-sm">启用 XTLS Vision 流控</span>
                  </div>
                )}
                {form.protocol === "shadowsocks" && (
                  <Select
                    label="加密方式"
                    selectedKeys={[form.method]}
                    onSelectionChange={(keys) =>
                      setForm((prev) => ({
                        ...prev,
                        method: String(Array.from(keys)[0] ?? prev.method),
                        password: "",
                      }))
                    }
                  >
                    {(caps?.ssMethods ?? []).map((m) => (
                      <SelectItem key={m} textValue={m}>
                        {m}
                      </SelectItem>
                    ))}
                  </Select>
                )}
                {(form.protocol === "trojan" ||
                  form.protocol === "shadowsocks") && (
                  <Input
                    description={
                      form.method.startsWith("2022-")
                        ? "SS2022 需要 base64 密钥，建议留空自动生成"
                        : undefined
                    }
                    label="密码"
                    placeholder="留空自动生成"
                    value={form.password}
                    variant="bordered"
                    onChange={(e) => set("password", e.target.value)}
                  />
                )}

                {(form.transportType === "ws" ||
                  form.transportType === "xhttp") && (
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <Input
                      label="路径"
                      placeholder="留空随机"
                      value={form.path}
                      variant="bordered"
                      onChange={(e) => set("path", e.target.value)}
                    />
                    <Input
                      label="Host（可选）"
                      value={form.host}
                      variant="bordered"
                      onChange={(e) => set("host", e.target.value)}
                    />
                  </div>
                )}
                {form.transportType === "xhttp" && (
                  <Select
                    label="XHTTP 模式"
                    selectedKeys={[form.xhttpMode]}
                    onSelectionChange={(keys) =>
                      set("xhttpMode", String(Array.from(keys)[0] ?? "auto"))
                    }
                  >
                    {XHTTP_MODES.map((m) => (
                      <SelectItem key={m} textValue={m}>
                        {m}
                      </SelectItem>
                    ))}
                  </Select>
                )}
                {form.transportType === "grpc" && (
                  <Input
                    label="gRPC serviceName"
                    placeholder="grpc"
                    value={form.serviceName}
                    variant="bordered"
                    onChange={(e) => set("serviceName", e.target.value)}
                  />
                )}

                {form.securityType === "reality" && (
                  <div className="space-y-3 rounded-xl border border-divider p-3">
                    <p className="text-sm font-medium">Reality</p>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                      <Input
                        description="客户端使用的 SNI，需是支持 TLS1.3 的大站"
                        label="伪装域名"
                        value={form.sni}
                        variant="bordered"
                        onChange={(e) => set("sni", e.target.value)}
                      />
                      <Input
                        description="留空则为 伪装域名:443"
                        label="目标地址 (dest)"
                        placeholder="www.example.com:443"
                        value={form.realityDest}
                        variant="bordered"
                        onChange={(e) => set("realityDest", e.target.value)}
                      />
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                      <Input
                        label="私钥"
                        placeholder="留空自动生成"
                        value={form.realityPrivateKey}
                        variant="bordered"
                        onChange={(e) =>
                          set("realityPrivateKey", e.target.value)
                        }
                      />
                      <Input
                        label="shortId"
                        placeholder="留空自动生成，多个用逗号分隔"
                        value={form.realityShortIds}
                        variant="bordered"
                        onChange={(e) => set("realityShortIds", e.target.value)}
                      />
                    </div>
                  </div>
                )}

                {form.securityType === "tls" && (
                  <div className="space-y-3 rounded-xl border border-divider p-3">
                    <p className="text-sm font-medium">TLS</p>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                      <Input
                        label="域名 (SNI)"
                        placeholder="node.example.com"
                        value={form.sni}
                        variant="bordered"
                        onChange={(e) => set("sni", e.target.value)}
                      />
                      <Select
                        label="证书"
                        selectedKeys={[form.certMode]}
                        onSelectionChange={(keys) =>
                          set("certMode", String(Array.from(keys)[0] ?? "self"))
                        }
                      >
                        <SelectItem key="self" textValue="自动生成自签证书">
                          自动生成自签证书
                        </SelectItem>
                        <SelectItem key="file" textValue="使用节点上的证书文件">
                          使用节点上的证书文件
                        </SelectItem>
                      </Select>
                    </div>
                    {form.certMode === "file" && (
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                        <Input
                          label="证书路径"
                          placeholder="/etc/ssl/fullchain.pem"
                          value={form.certFile}
                          variant="bordered"
                          onChange={(e) => set("certFile", e.target.value)}
                        />
                        <Input
                          label="私钥路径"
                          placeholder="/etc/ssl/privkey.pem"
                          value={form.keyFile}
                          variant="bordered"
                          onChange={(e) => set("keyFile", e.target.value)}
                        />
                      </div>
                    )}
                    <Input
                      label="ALPN（可选）"
                      placeholder="h2,http/1.1"
                      value={form.alpn}
                      variant="bordered"
                      onChange={(e) => set("alpn", e.target.value)}
                    />
                  </div>
                )}

                <button
                  className="text-sm text-primary"
                  type="button"
                  onClick={() => setShowAdvanced((v) => !v)}
                >
                  {showAdvanced ? "收起高级选项" : "展开高级选项"}
                </button>
                {showAdvanced && (
                  <div className="space-y-3">
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                      <Input
                        description="默认 :: 监听所有地址；127.0.0.1 仅本机（配合隧道中转）"
                        label="监听地址"
                        value={form.listen}
                        variant="bordered"
                        onChange={(e) => set("listen", e.target.value)}
                      />
                      <Input
                        description="客户端连接的地址，留空使用节点 IP"
                        label="分享地址"
                        value={form.shareHost}
                        variant="bordered"
                        onChange={(e) => set("shareHost", e.target.value)}
                      />
                    </div>
                    <Input
                      label="备注"
                      value={form.remark}
                      variant="bordered"
                      onChange={(e) => set("remark", e.target.value)}
                    />
                    <Textarea
                      description="内核原生字段，深度合并到该入站配置中（例如 sniffing、sockopt）"
                      label="高级配置 (JSON)"
                      minRows={3}
                      placeholder='{"streamSettings":{"sockopt":{"tcpFastOpen":true}}}'
                      value={form.extra}
                      variant="bordered"
                      onChange={(e) => set("extra", e.target.value)}
                    />
                  </div>
                )}
              </div>
            </ModalBody>
            <ModalFooter>
              <Button variant="light" onPress={onClose}>
                取消
              </Button>
              <Button color="primary" isLoading={saving} onPress={submit}>
                {isEdit ? "保存并下发" : "创建并下发"}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
