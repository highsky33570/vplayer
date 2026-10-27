import { useEffect, useState } from 'react';
import { createMembershipOrder, listMembershipPlans, type MembershipPlan } from '../api';
import { useAuth } from '../auth/AuthContext';

function formatPrice(usd: number): string {
  if (Number.isInteger(usd)) return `$${usd}`;
  return `$${usd.toFixed(2)}`;
}

function planHint(days: number): string {
  if (days >= 365) return '全年畅看';
  if (days >= 180) return '超值半年';
  if (days >= 90) return '季度特惠';
  return '按月开通';
}

export function VipPurchasePanel() {
  const { user, token, ready, openLogin } = useAuth();
  const [plans, setPlans] = useState<MembershipPlan[]>([]);
  const [selected, setSelected] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    listMembershipPlans()
      .then((rows) => {
        if (cancelled) return;
        setPlans(rows);
        if (rows.length > 0) setSelected(rows[0].slug);
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : '加载套餐失败');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const isMember = Boolean(user?.is_member || user?.is_admin);
  const daysLeft = user?.membership_days_remaining ?? 0;

  const onPay = async () => {
    setError('');
    if (!ready) return;
    if (!user || !token) {
      openLogin('login');
      return;
    }
    if (!selected) {
      setError('请选择套餐');
      return;
    }
    setBusy(true);
    try {
      const order = await createMembershipOrder(token, selected);
      if (!order.payment_url) {
        throw new Error('未返回支付链接');
      }
      window.location.assign(order.payment_url);
    } catch (e) {
      setError(e instanceof Error ? e.message : '创建订单失败');
      setBusy(false);
    }
  };

  return (
    <section className="vip-purchase" aria-label="开通大会员">
      <div className="vip-purchase-head">
        <h2>开通大会员</h2>
        <p>仅支持加密货币（USDT）支付。支付完成后自动开通，可在账户页查看剩余天数。</p>
        {isMember ? (
          <p className="vip-purchase-active">
            {user?.is_admin
              ? '当前为管理员，已具备大会员权限。'
              : daysLeft > 0
                ? `当前已是大会员，剩余 ${daysLeft} 天。续费将在现有到期日上叠加。`
                : '当前已是大会员。'}
          </p>
        ) : null}
      </div>

      {loading ? <p className="me-status">加载套餐…</p> : null}
      {error ? (
        <p className="me-feedback is-error" role="alert">
          {error}
        </p>
      ) : null}

      {!loading && plans.length > 0 ? (
        <div className="vip-plan-grid" role="radiogroup" aria-label="会员套餐">
          {plans.map((plan) => {
            const active = selected === plan.slug;
            return (
              <button
                key={plan.slug}
                type="button"
                role="radio"
                aria-checked={active}
                className={`vip-plan-card${active ? ' is-selected' : ''}`}
                onClick={() => setSelected(plan.slug)}
              >
                <span className="vip-plan-name">{plan.name}</span>
                <strong className="vip-plan-price">{formatPrice(plan.price_usd)}</strong>
                <span className="vip-plan-meta">{plan.duration_days} 天 · {planHint(plan.duration_days)}</span>
              </button>
            );
          })}
        </div>
      ) : null}

      <div className="vip-purchase-actions">
        <button type="button" className="vip-cta" disabled={busy || loading || !selected} onClick={() => void onPay()}>
          {busy ? '正在跳转支付…' : user ? '去支付（USDT）' : '登录后开通'}
        </button>
      </div>
    </section>
  );
}
