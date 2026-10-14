import type {
  InboundApiItem,
  InboundMutationPayload,
  KernelCapabilitiesApiData,
  KernelProtocolCap,
} from "@/api/types";

export const PROTOCOL_LABELS: Record<string, string> = {
  vless: "VLESS",
  vmess: "VMess",
  trojan: "Trojan",
  shadowsocks: "Shadowsocks",
};

export const TRANSPORT_LABELS: Record<string, string> = {
  tcp: "TCP",
  ws: "WebSocket",
  grpc: "gRPC",
  xhttp: "XHTTP",
};

export const SECURITY_LABELS: Record<string, string> = {
  none: "无",
  tls: "TLS",
  reality: "Reality",
};

export const KERNEL_STATUS: Record<
  string,
  { label: string; color: "success" | "warning" | "danger" | "default" }
> = {
  running: { label: "运行中", color: "success" },
  stopped: { label: "已停止", color: "default" },
  pending: { label: "待下发", color: "warning" },
  error: { label: "异常", color: "danger" },
};

export interface InboundForm {
  id?: number;
  nodeId: string;
  kernelType: string;
  name: string;
  protocol: string;
  listen: string;
  port: string;
  uuid: string;
  flowVision: boolean;
  password: string;
  method: string;
  transportType: string;
  path: string;
  host: string;
  serviceName: string;
  xhttpMode: string;
  securityType: string;
  sni: string;
  alpn: string;
  fingerprint: string;
  certMode: string;
  certFile: string;
  keyFile: string;
  realityDest: string;
  realityPrivateKey: string;
  realityShortIds: string;
  shareHost: string;
  remark: string;
  extra: string;
  relayOnly: boolean;
  acceptProxyProtocol: boolean;
  relayWhitelist: boolean;
  whitelistExtra: string;
  enabled: boolean;
}

export interface Preset {
  key: string;
  label: string;
  description: string;
  protocol: string;
  transport: string;
  security: string;
  vision?: boolean;
  needsDomain?: boolean;
}

export const PRESETS: Preset[] = [
  {
    key: "vless-reality-vision",
    label: "VLESS + Reality + Vision",
    description: "推荐：无需域名和证书，抗探测",
    protocol: "vless",
    transport: "tcp",
    security: "reality",
    vision: true,
  },
  {
    key: "vless-xhttp-reality",
    label: "VLESS + XHTTP + Reality",
    description: "XHTTP 传输，适合中转/CDN 场景",
    protocol: "vless",
    transport: "xhttp",
    security: "reality",
  },
  {
    key: "ss2022",
    label: "Shadowsocks 2022",
    description: "轻量，客户端支持广泛",
    protocol: "shadowsocks",
    transport: "tcp",
    security: "none",
  },
  {
    key: "trojan-tls",
    label: "Trojan + TLS（自签）",
    description: "需要填写域名，自动生成自签证书",
    protocol: "trojan",
    transport: "tcp",
    security: "tls",
    needsDomain: true,
  },
  {
    key: "vmess-ws-tls",
    label: "VMess + WS + TLS",
    description: "兼容老客户端",
    protocol: "vmess",
    transport: "ws",
    security: "tls",
    needsDomain: true,
  },
];

export function emptyForm(
  caps: KernelCapabilitiesApiData | null,
  nodeId = "",
): InboundForm {
  return {
    nodeId,
    kernelType: "xray",
    name: "",
    protocol: "vless",
    listen: "::",
    port: "",
    uuid: "",
    flowVision: true,
    password: "",
    method: caps?.defaults.ssMethod ?? "2022-blake3-aes-128-gcm",
    transportType: "tcp",
    path: "",
    host: "",
    serviceName: "",
    xhttpMode: "auto",
    securityType: "reality",
    sni: caps?.defaults.realitySni ?? "",
    alpn: "",
    fingerprint: caps?.defaults.fingerprint ?? "chrome",
    certMode: "self",
    certFile: "",
    keyFile: "",
    realityDest: "",
    realityPrivateKey: "",
    realityShortIds: "",
    shareHost: "",
    remark: "",
    extra: "",
    relayOnly: false,
    acceptProxyProtocol: false,
    relayWhitelist: false,
    whitelistExtra: "",
    enabled: true,
  };
}

export function applyPreset(
  form: InboundForm,
  preset: Preset,
  caps: KernelCapabilitiesApiData | null,
): InboundForm {
  const realitySni = caps?.defaults.realitySni ?? "";

  return {
    ...form,
    protocol: preset.protocol,
    transportType: preset.transport,
    securityType: preset.security,
    flowVision: Boolean(preset.vision),
    sni:
      preset.security === "reality"
        ? form.sni && form.securityType === "reality"
          ? form.sni
          : realitySni
        : form.securityType === "reality"
          ? ""
          : form.sni,
    name: form.name || preset.label,
  };
}

export function formFromItem(item: InboundApiItem): InboundForm {
  return {
    id: item.id,
    nodeId: String(item.nodeId),
    kernelType: item.kernelType,
    name: item.name,
    protocol: item.protocol,
    listen: item.listen,
    port: String(item.port),
    uuid: item.settings.uuid ?? "",
    flowVision: Boolean(item.settings.flow),
    password: item.settings.password ?? "",
    method: item.settings.method ?? "",
    transportType: item.transport.type || "tcp",
    path: item.transport.path ?? "",
    host: item.transport.host ?? "",
    serviceName: item.transport.serviceName ?? "",
    xhttpMode: item.transport.mode || "auto",
    securityType: item.security.type || "none",
    sni: item.security.sni ?? "",
    alpn: (item.security.alpn ?? []).join(","),
    fingerprint: item.security.fingerprint ?? "",
    certMode: item.security.certMode || "self",
    certFile: item.security.certFile ?? "",
    keyFile: item.security.keyFile ?? "",
    realityDest: item.security.realityDest ?? "",
    realityPrivateKey: item.security.realityPrivateKey ?? "",
    realityShortIds: (item.security.realityShortIds ?? []).join(","),
    shareHost: item.shareHost,
    remark: item.remark,
    extra: item.extra,
    relayOnly: item.relayOnly,
    acceptProxyProtocol: item.acceptProxyProtocol,
    relayWhitelist: item.relayWhitelist,
    whitelistExtra: item.whitelistExtra,
    enabled: item.enabled,
  };
}

function splitList(v: string): string[] {
  return v
    .split(/[,，\s]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

export function visionAllowed(form: InboundForm): boolean {
  return (
    form.protocol === "vless" &&
    form.transportType === "tcp" &&
    form.securityType !== "none"
  );
}

export function toPayload(form: InboundForm): InboundMutationPayload {
  const usesUUID = form.protocol === "vless" || form.protocol === "vmess";
  const usesPassword =
    form.protocol === "trojan" || form.protocol === "shadowsocks";

  return {
    id: form.id,
    nodeId: form.nodeId ? Number(form.nodeId) : undefined,
    kernelType: form.kernelType,
    name: form.name.trim(),
    protocol: form.protocol,
    listen: form.listen.trim(),
    port: form.port.trim() ? Number(form.port) : 0,
    settings: {
      uuid: usesUUID ? form.uuid.trim() : undefined,
      flow:
        visionAllowed(form) && form.flowVision ? "xtls-rprx-vision" : undefined,
      password: usesPassword ? form.password.trim() : undefined,
      method: form.protocol === "shadowsocks" ? form.method : undefined,
    },
    transport: {
      type: form.transportType,
      path: form.path.trim() || undefined,
      host: form.host.trim() || undefined,
      serviceName: form.serviceName.trim() || undefined,
      mode: form.transportType === "xhttp" ? form.xhttpMode : undefined,
    },
    security: {
      type: form.securityType,
      sni: form.sni.trim() || undefined,
      alpn: form.alpn.trim() ? splitList(form.alpn) : undefined,
      fingerprint: form.fingerprint.trim() || undefined,
      certMode: form.securityType === "tls" ? form.certMode : undefined,
      certFile: form.certFile.trim() || undefined,
      keyFile: form.keyFile.trim() || undefined,
      realityDest: form.realityDest.trim() || undefined,
      realityPrivateKey: form.realityPrivateKey.trim() || undefined,
      realityShortIds: form.realityShortIds.trim()
        ? splitList(form.realityShortIds)
        : undefined,
    },
    extra: form.extra.trim(),
    shareHost: form.shareHost.trim(),
    remark: form.remark.trim(),
    relayOnly: form.relayOnly || form.acceptProxyProtocol,
    acceptProxyProtocol: form.acceptProxyProtocol,
    relayWhitelist: form.relayWhitelist,
    whitelistExtra: form.relayWhitelist ? form.whitelistExtra.trim() : "",
    enabled: form.enabled,
  };
}

export function protocolCap(
  caps: KernelCapabilitiesApiData | null,
  kernelType: string,
  protocol: string,
): KernelProtocolCap | undefined {
  return caps?.kernels
    .find((k) => k.type === kernelType)
    ?.protocols?.find((p) => p.protocol === protocol);
}

export function describeInbound(item: InboundApiItem): string {
  const parts = [PROTOCOL_LABELS[item.protocol] ?? item.protocol];

  if (item.protocol !== "shadowsocks") {
    parts.push(TRANSPORT_LABELS[item.transport.type] ?? item.transport.type);
  }
  if (item.security.type && item.security.type !== "none") {
    parts.push(SECURITY_LABELS[item.security.type] ?? item.security.type);
  }
  if (item.settings.flow) {
    parts.push("Vision");
  }

  return parts.join(" + ");
}
