import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = 'http://3.238.87.0';

export const options = {
    scenarios: {
        SendMessage: {
            executor: 'constant-vus',
            vus: 10,
            duration: '20s',
            exec: 'sendMessage'
        },
    }
}

export function sendMessage() {
    const url = `${BASE_URL}:7072/send/nt`

    const headers = {
        'Content-Type': 'application/json',
    }

    const payload = JSON.stringify({
        device_id: "68bb295f7670ccfbc643551b",
        number: '5511945106709',
        message: `Usuario Virtual ${__VU}`
    });

    const res = http.post(url, payload, { headers });

    check(res, {
        'message status 200': (r) => r.status === 200,
    });

    sleep(1);
}