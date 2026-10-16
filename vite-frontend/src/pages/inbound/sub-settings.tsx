import type { SubPreviewItem, SubSettings } from "@/api";

import { useEffect, useState } from "react";
import toast from "react-hot-toast";

import { getSubSettings, previewSubSettings, saveSubSettings } from "@/api";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Card, CardBody } from "@/shadcn-bridge/heroui/card";
import { Input, Textarea } from "@/shadcn-bridge/heroui/input";
import { Switch } from "@/shadcn-bridge/heroui/switch";

const EMPTY: SubSettings = {
  rename: "",
  prefix: "",
  sort: "",
  regionGroups: false,
};

const RENAME_EXAMPLE = `花云| =>
/\\|[0-9.]+x/ =>
| => -`;

/** Node naming, order and region groups for every subscription. */
export function SubSettingsCard() {
  const [saved, setSaved] = useState<SubSettings>(EMPTY);
  const [form, setForm] = useState<SubSettings>(EMPTY);
  const [saving, setSaving] = useState(false);
  const [preview, setPreview] = useState<SubPreviewItem[] | null>(null);
  const [previewing, setPreviewing] = useState(false);

  useEffect(() => {
    getSubSettings().then((res) => {
      if (res.code === 0 && res.data) {
        setSaved(res.data);
        setForm(res.data);
      }
    });
  }, []);

  const dirty = JSON.stringify(form) !== JSON.stringify(saved);
  const set = (patch: Partial<SubSettings>) => {
    setForm((f) => ({ ...f, ...patch }));
    setPreview(null);
  };

  const save = async () => {
    setSaving(true);
    try {
      const res = await saveSubSettings(form);

      if (res.code !== 0) {
        toast.error(res.msg || "保存失败");

        return;
      }
      setSaved(res.data);
      setForm(res.data);
      toast.success("已保存，客户端下次更新订阅时生效");
    } finally {
      setSaving(false);
    }
  };

  const runPreview = async () => {
    setPreviewing(true);
    try {
      const res = await previewSubSettings(form);

      if (res.code !== 0) {
        toast.error(res.msg || "预览失败");

        return;
      }
      setPreview(res.data ?? []);
    } finally {
      setPreviewing(false);
    }
  };

  return (
    <Card className="shadow-sm">
      <CardBody className="space-y-4">
        <div>
          <h3 className="text-base font-semibold">订阅设置</h3>
          <p className="text-xs text-default-500">
            改名、排序和按地区分组，对你的订阅和共享用户的订阅都生效
          </p>
        </div>

        <div className="space-y-1.5">
          <p className="text-sm font-medium">改名规则</p>
          <p className="text-xs text-default-500">
            每行一条：<code>要替换的 =&gt; 替换成</code>，右边留空就是删除；用{" "}
            <code>/.../</code> 包起来是正则。按顺序执行，# 开头是注释
          </p>
          <Textarea
            classNames={{ input: "font-mono text-xs" }}
            minRows={3}
            placeholder={RENAME_EXAMPLE}
            value={form.rename}
            variant="bordered"
            onChange={(e) => set({ rename: e.target.value })}
          />
        </div>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div className="space-y-1.5">
            <p className="text-sm font-medium">名称前缀</p>
            <Input
              placeholder="例如 [我的] "
              value={form.prefix}
              variant="bordered"
              onChange={(e) => set({ prefix: e.target.value })}
            />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium" htmlFor="sub-sort">
              排序
            </label>
            <select
              className="h-10 w-full rounded-lg border border-divider bg-background px-2 text-sm"
              id="sub-sort"
              value={form.sort}
              onChange={(e) => set({ sort: e.target.value })}
            >
              <option value="">按面板里的顺序</option>
              <option value="name">按名称（数字按大小）</option>
            </select>
          </div>
        </div>

        <div className="flex items-start gap-3">
          <Switch
            aria-labelledby="sub-region-label"
            id="sub-region-switch"
            isSelected={form.regionGroups}
            size="sm"
            onValueChange={(on) => set({ regionGroups: on })}
          />
          <label
            className="flex cursor-pointer flex-col"
            htmlFor="sub-region-switch"
            id="sub-region-label"
          >
            <span className="text-sm">按地区自动分组</span>
            <span className="text-xs text-default-500">
              Clash / sing-box / Surge
              里按名称识别香港、日本、美国等，每个地区一个自动测速组
            </span>
          </label>
        </div>

        <div className="flex flex-wrap justify-end gap-2">
          <Button isLoading={previewing} variant="flat" onPress={runPreview}>
            预览
          </Button>
          <Button
            color="primary"
            isDisabled={!dirty}
            isLoading={saving}
            onPress={save}
          >
            保存
          </Button>
        </div>

        {preview && (
          <div className="max-h-72 overflow-y-auto rounded-xl border border-divider">
            {preview.length === 0 ? (
              <p className="p-3 text-xs text-default-500">订阅里还没有节点</p>
            ) : (
              preview.map((p, i) => (
                <div
                  key={`${p.original}-${i}`}
                  className="flex items-center gap-2 border-b border-divider px-3 py-1.5 text-xs last:border-b-0"
                >
                  <span className="min-w-0 flex-1 truncate">
                    {p.name}
                    {p.name !== p.original && (
                      <span className="text-default-400"> ← {p.original}</span>
                    )}
                  </span>
                  {form.regionGroups && (
                    <span className="shrink-0 text-default-500">
                      {p.region || "未分组"}
                    </span>
                  )}
                </div>
              ))
            )}
          </div>
        )}
      </CardBody>
    </Card>
  );
}
