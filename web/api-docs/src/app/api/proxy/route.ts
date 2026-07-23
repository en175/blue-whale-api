import { openapi } from '@/lib/openapi';

export const { GET, HEAD, PUT, POST, PATCH, DELETE } = openapi.createProxy({
  // No target domain restriction, allowing users to test any API address
  // Note: This means the proxy will forward requests to any user-specified address
});
