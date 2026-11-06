import {
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

interface FlowChartPoint {
  time: string;
  flow: number;
}

interface FlowChartCardProps {
  statisticsFlowsCount: number;
  chartData: FlowChartPoint[];
  formatFlow: (value: number, unit?: string) => string;
}

export const FlowChartCard = ({
  statisticsFlowsCount,
  chartData,
  formatFlow,
}: FlowChartCardProps) => {
  return (
    <section className="mb-8">
      <h2 className="border-b border-divider pb-2 text-sm font-semibold">
        近 24 小时流量
      </h2>
      <div className="pt-4">
        {statisticsFlowsCount === 0 ? (
          <p className="py-6 text-sm text-default-400">暂无流量统计数据</p>
        ) : (
          <div className="h-56 w-full lg:h-64">
            <ResponsiveContainer height="100%" width="100%">
              <LineChart
                data={chartData}
                margin={{ top: 20, right: 0, left: -20, bottom: 0 }}
              >
                <XAxis
                  axisLine={false}
                  dataKey="time"
                  tick={{ fill: "currentColor", opacity: 0.4, fontSize: 11 }}
                  tickLine={false}
                />
                <YAxis
                  axisLine={false}
                  tick={{ fill: "currentColor", opacity: 0.4, fontSize: 11 }}
                  tickFormatter={(value: number | string) => {
                    const v = typeof value === "number" ? value : Number(value);

                    if (!Number.isFinite(v)) return String(value);
                    if (v === 0) return "0";
                    if (v < 1024) return `${v}B`;
                    if (v < 1024 * 1024) return `${(v / 1024).toFixed(1)}K`;
                    if (v < 1024 * 1024 * 1024)
                      return `${(v / (1024 * 1024)).toFixed(1)}M`;

                    return `${(v / (1024 * 1024 * 1024)).toFixed(1)}G`;
                  }}
                  tickLine={false}
                />
                <Tooltip
                  content={({
                    active,
                    payload,
                    label,
                  }: {
                    active?: boolean;
                    payload?: Array<{ value?: number | string }>;
                    label?: string | number;
                  }) => {
                    if (active && payload && payload.length) {
                      const firstValue = payload[0]?.value;
                      const numericValue =
                        typeof firstValue === "number"
                          ? firstValue
                          : Number(firstValue);
                      const flowValue = Number.isFinite(numericValue)
                        ? numericValue
                        : 0;

                      return (
                        <div className="rounded-md border border-divider bg-background px-2.5 py-1.5 text-xs">
                          <p className="text-default-400">{label ?? ""}</p>
                          <p className="tabular-nums text-foreground">
                            {formatFlow(flowValue)}
                          </p>
                        </div>
                      );
                    }

                    return null;
                  }}
                  cursor={{ stroke: "currentColor", strokeOpacity: 0.15 }}
                />
                <Line
                  activeDot={{ r: 3, strokeWidth: 0, fill: "currentColor" }}
                  dataKey="flow"
                  dot={false}
                  stroke="currentColor"
                  strokeWidth={1.5}
                  type="monotone"
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>
    </section>
  );
};
