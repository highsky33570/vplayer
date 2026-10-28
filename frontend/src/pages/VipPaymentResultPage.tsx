import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { getMembershipOrder, type MembershipOrder } from '../api';
import { AuthGate } from '../auth/AuthGate';

export function VipPaymentResultPage() {
  return <AuthGate>{({ token }) => <VipPaymentResultInner token={token} />}</AuthGate>;
}

function VipPaymentResultInner({ token }: { token: string }) {
  const [params] = useSearchParams();
  const orderNo = (params.get('order') ?? '').trim();
  const [order, setOrder] = useState<MembershipOrder | null>(null);
  const [error, setError] = useState('');
  const [polling, setPolling] = useState(true);

  useEffect(() => {
    if (!orderNo) {
      setError('缺少订单号');
      setPolling(false);
      return;
    }
    let cancelled = false;
    let tries = 0;
    const tick = async () => {
      try {
        const o = await getMembershipOrder(token, orderNo);
        if (cancelled) return;
        setOrder(o);
        if (o.status === 'paid') {
          setPolling(false);
          return;
        }
        if (o.status === 'expired' || o.status === 'cancelled') {
          setPolling(false);
          return;
        }
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : '查询订单失败');
      }
      tries += 1;
      if (tries >= 40) {
        if (!cancelled) setPolling(false);
        return;
      }
      if (!cancelled) window.setTimeout(() => void tick(), 3000);
    };
    void tick();
    return () => {
      cancelled = true;
    };
  }, [orderNo, token]);

  const payUrl = (order?.payment_url ?? '').trim();
  const pending = order?.status === 'pending';

  return (
    <div className="page vip-pay-result">
      <h1>支付结果</h1>
      <p className="vip-pay-hint">
        支付在 GM Pay 页面完成。可随时关闭支付页，回到本站查看开通状态。
      </p>
      {!orderNo ? <p className="me-feedback is-error">缺少订单号，请从大会员页重新发起支付。</p> : null}
      {error ? (
        <p className="me-feedback is-error" role="alert">
          {error}
        </p>
      ) : null}
      {order ? (
        <div className="vip-pay-card">
          <p>
            订单号：<code>{order.order_no}</code>
          </p>
          <p>
            套餐：{order.plan_name}（{order.duration_days} 天）· ${order.amount_usd}
          </p>
          <p>
            状态：
            <strong>
              {order.status === 'paid'
                ? '已支付并开通'
                : order.status === 'pending'
                  ? '等待链上确认'
                  : order.status}
            </strong>
          </p>
          {order.membership_granted_until ? (
            <p>会员有效至：{new Date(order.membership_granted_until).toLocaleString()}</p>
          ) : null}
          {polling ? <p className="me-status">正在确认支付，请稍候…</p> : null}
          {order.status === 'paid' ? (
            <p className="me-feedback">开通成功。可在账户页查看大会员徽章与剩余天数。</p>
          ) : null}
        </div>
      ) : polling ? (
        <p className="me-status">正在查询订单…</p>
      ) : null}
      <div className="vip-purchase-actions">
        {pending && payUrl ? (
          <a className="vip-cta" href={payUrl} target="_blank" rel="noopener noreferrer">
            打开支付页面
          </a>
        ) : null}
        <Link className="vip-cta vip-cta--ghost" to="/vip">
          返回大会员
        </Link>
        <Link className="vip-cta vip-cta--ghost" to="/account">
          我的账户
        </Link>
      </div>
    </div>
  );
}
